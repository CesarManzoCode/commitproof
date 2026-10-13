package workload

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
)

// Consumer is W7 (CONTRACT §7, ADR-0029, ADR-0030): a logical replication
// consumer of a non-temporary failover slot through one gateway. It records
// every connection, every received change (at its transaction's COMMIT,
// with the commit end LSN), every standby status update and every stream
// end through Record; it reconnects after any error with the same slot,
// starting at its last confirmed flush.
type Consumer struct {
	GW, Slot, Publication string
	Timeout               time.Duration
	Port                  int
	// Record writes one operation: invoke now, completion when done is
	// called (the runner's recorder; one logical process).
	Record func(f string, in map[string]any) (done func(Outcome))
	// StatusInterval is W7's standby status update period (1 s).
	StatusInterval time.Duration

	mu        sync.Mutex
	conn      int64
	created   bool
	createErr error
	flushed   uint64 // last confirmed flush (commit end LSN)
	received  uint64 // highest position seen: commit end LSNs, server WAL ends
	target    uint64
	catchUp   chan struct{} // closed when received ≥ target
	slotReady chan struct{} // closed once the slot exists and streaming began
	readyOnce sync.Once
}

// Ready is closed when the first connection created the slot and started
// streaming.
func (c *Consumer) Ready() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.slotReady == nil {
		c.slotReady = make(chan struct{})
	}
	return c.slotReady
}

// CreateErr is the slot creation error of the last attempt (nil if none).
func (c *Consumer) CreateErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.createErr
}

// Run streams until ctx ends, reconnecting 1 s after every error.
func (c *Consumer) Run(ctx context.Context) {
	c.Ready()
	for ctx.Err() == nil {
		c.stream(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// CatchUp asks the stream to report when everything up to target (the
// server's WAL position after the final reads) has been received, and waits
// up to deadline. It records op caught-up.
func (c *Consumer) CatchUp(ctx context.Context, target uint64, deadline time.Duration) bool {
	c.mu.Lock()
	conn := c.conn
	c.target = target
	ch := make(chan struct{})
	c.catchUp = ch
	reached := c.received >= target
	c.mu.Unlock()
	done := c.Record("caught-up", map[string]any{"conn": conn, "target_lsn": int64(target)})
	if !reached {
		select {
		case <-ch:
			reached = true
		case <-time.After(deadline):
		case <-ctx.Done():
		}
	}
	c.mu.Lock()
	recv, conn := c.received, c.conn
	c.mu.Unlock()
	if !reached {
		a := Attempt{Autocommit: true, At: PhaseStatement, ClientTimeout: true, Err: fmt.Errorf("not caught up: received %s, target %s", pglogrepl.LSN(recv), pglogrepl.LSN(target))}
		done(finishAttempt(a, nil))
		return false
	}
	done(Outcome{Type: OK, Value: map[string]any{"conn": conn, "target_lsn": int64(target), "received_lsn": int64(recv)}})
	return true
}

func (c *Consumer) dsn() string {
	port := GatewayPort
	if c.Port != 0 {
		port = c.Port
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable replication=database application_name=commitproof-cdc",
		c.GW, port, DBUser, DBPassword, DBName)
}

func (c *Consumer) advance(pos uint64) {
	if pos > c.received {
		c.received = pos
	}
	if c.catchUp != nil && c.received >= c.target {
		close(c.catchUp)
		c.catchUp = nil
	}
}

type relation struct {
	table string
	cols  []string
}

type change struct {
	table string
	row   map[string]any
}

// stream is one connection: connect (and create the slot the first time),
// START_REPLICATION at the last confirmed flush, then receive until an
// error.
func (c *Consumer) stream(ctx context.Context) {
	c.mu.Lock()
	c.conn++
	conn, start, first := c.conn, c.flushed, !c.created
	c.mu.Unlock()
	done := c.Record("connect", map[string]any{"conn": conn})
	a := Attempt{Autocommit: true, At: PhaseConnect}
	cctx, cancel := context.WithTimeout(ctx, c.Timeout)
	pc, err := pgconn.Connect(cctx, c.dsn())
	if err == nil && first {
		a.At = PhaseStatement
		_, err = pglogrepl.ParseCreateReplicationSlot(pc.Exec(cctx, fmt.Sprintf("CREATE_REPLICATION_SLOT %s LOGICAL pgoutput (SNAPSHOT 'nothing', FAILOVER true)", c.Slot)))
		c.mu.Lock()
		c.createErr = err
		if err == nil {
			c.created = true
		}
		c.mu.Unlock()
	}
	if err == nil {
		a.At = PhaseStatement
		err = pglogrepl.StartReplication(cctx, pc, c.Slot, pglogrepl.LSN(start), pglogrepl.StartReplicationOptions{
			Mode: pglogrepl.LogicalReplication, PluginArgs: []string{"proto_version '1'", "publication_names '" + c.Publication + "'"}})
	}
	cancel()
	if err != nil {
		fill(&a, err, ctx)
		done(finishAttempt(a, nil))
		if pc != nil {
			closeQuietly(pc)
		}
		return
	}
	done(Outcome{Type: OK, Value: map[string]any{"conn": conn, "slot": c.Slot, "start_lsn": int64(start)}})
	c.readyOnce.Do(func() {
		c.mu.Lock()
		close(c.slotReady)
		c.mu.Unlock()
	})
	err = c.receive(ctx, pc, conn)
	closeQuietly(pc)
	if ctx.Err() != nil {
		return
	}
	end := c.Record("stream-end", map[string]any{"conn": conn})
	ea := Attempt{Autocommit: true, At: PhaseStatement, Transport: true}
	fill(&ea, err, ctx)
	end(finishAttempt(ea, nil))
}

func closeQuietly(pc *pgconn.PgConn) {
	cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	pc.Close(cctx)
	cancel()
}

func (c *Consumer) sendStatus(pc *pgconn.PgConn, conn int64, reply bool) error {
	c.mu.Lock()
	flush := c.flushed
	c.mu.Unlock()
	done := c.Record("ack", map[string]any{"conn": conn, "flush": int64(flush)})
	err := pglogrepl.SendStandbyStatusUpdate(context.Background(), pc, pglogrepl.StandbyStatusUpdate{
		WALWritePosition: pglogrepl.LSN(flush), WALFlushPosition: pglogrepl.LSN(flush), WALApplyPosition: pglogrepl.LSN(flush), ReplyRequested: reply})
	if err != nil {
		a := Attempt{Autocommit: true, At: PhaseStatement, Transport: true}
		fill(&a, err, context.Background())
		done(finishAttempt(a, nil))
		return err
	}
	done(Outcome{Type: OK, Value: map[string]any{"conn": conn, "flush": int64(flush)}})
	return nil
}

// receive handles XLogData and keepalives until an error or ctx ends.
func (c *Consumer) receive(ctx context.Context, pc *pgconn.PgConn, conn int64) error {
	rels := map[uint32]relation{}
	var pending []change
	var xid uint32
	interval := c.StatusInterval
	if interval <= 0 {
		interval = time.Second
	}
	next := time.Now().Add(interval)
	for {
		if time.Now().After(next) {
			c.mu.Lock()
			reply := c.catchUp != nil
			c.mu.Unlock()
			if err := c.sendStatus(pc, conn, reply); err != nil {
				return err
			}
			next = time.Now().Add(interval)
		}
		rctx, cancel := context.WithDeadline(ctx, next)
		msg, err := pc.ReceiveMessage(rctx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if pgconn.Timeout(err) && !pc.IsClosed() {
				continue
			}
			return err
		}
		switch m := msg.(type) {
		case *pgproto3.ErrorResponse:
			return pgconn.ErrorResponseToPgError(m)
		case *pgproto3.CopyData:
			if len(m.Data) == 0 {
				continue
			}
			switch m.Data[0] {
			case pglogrepl.PrimaryKeepaliveMessageByteID:
				k, err := pglogrepl.ParsePrimaryKeepaliveMessage(m.Data[1:])
				if err != nil {
					return err
				}
				c.mu.Lock()
				c.advance(uint64(k.ServerWALEnd))
				c.mu.Unlock()
				if k.ReplyRequested {
					next = time.Now()
				}
			case pglogrepl.XLogDataByteID:
				x, err := pglogrepl.ParseXLogData(m.Data[1:])
				if err != nil {
					return err
				}
				lm, err := pglogrepl.Parse(x.WALData)
				if err != nil {
					return err
				}
				switch lm := lm.(type) {
				case *pglogrepl.RelationMessage:
					r := relation{table: lm.Namespace + "." + lm.RelationName}
					for _, col := range lm.Columns {
						r.cols = append(r.cols, col.Name)
					}
					rels[lm.RelationID] = r
				case *pglogrepl.BeginMessage:
					pending, xid = nil, lm.Xid
				case *pglogrepl.InsertMessage:
					ch, err := decodeChange(rels, lm.RelationID, lm.Tuple)
					if err != nil {
						return err
					}
					pending = append(pending, ch)
				case *pglogrepl.UpdateMessage:
					ch, err := decodeChange(rels, lm.RelationID, lm.NewTuple)
					if err != nil {
						return err
					}
					pending = append(pending, ch)
				case *pglogrepl.CommitMessage:
					end := uint64(lm.TransactionEndLSN)
					c.mu.Lock()
					flushed := c.flushed
					c.mu.Unlock()
					for _, ch := range pending {
						d := c.Record("recv", map[string]any{"conn": conn})
						d(Outcome{Type: OK, Value: map[string]any{"conn": conn, "table": ch.table, "row": ch.row,
							"lsn": int64(end), "xid": int64(xid), "flushed": int64(flushed)}})
					}
					pending = nil
					c.mu.Lock()
					c.flushed = end
					c.advance(end)
					c.mu.Unlock()
				}
			}
		}
	}
}

// decodeChange reads a cp.ledger ({id}) or cp.append ({k, vals}) row.
func decodeChange(rels map[uint32]relation, id uint32, t *pglogrepl.TupleData) (change, error) {
	r, ok := rels[id]
	if !ok || t == nil {
		return change{}, fmt.Errorf("change of unknown relation %d", id)
	}
	cols := make([][]byte, len(t.Columns))
	for i, col := range t.Columns {
		if col.DataType == pglogrepl.TupleDataTypeText {
			cols[i] = col.Data
		}
	}
	v := TupleText(r.cols, cols)
	switch r.table {
	case "cp.ledger":
		n, err := strconv.ParseInt(v["id"], 10, 64)
		if err != nil {
			return change{}, fmt.Errorf("cp.ledger id %q: %w", v["id"], err)
		}
		return change{r.table, map[string]any{"id": n}}, nil
	case "cp.append":
		k, err := strconv.ParseInt(v["k"], 10, 64)
		if err != nil {
			return change{}, fmt.Errorf("cp.append k %q: %w", v["k"], err)
		}
		vals, err := ParseBigintArray(v["vals"])
		if err != nil {
			return change{}, err
		}
		l := make([]any, len(vals))
		for i, x := range vals {
			l[i] = x
		}
		return change{r.table, map[string]any{"k": k, "vals": l}}, nil
	}
	return change{}, errors.New("change of unpublished table " + r.table)
}

// ServerWALPosition reads pg_current_wal_lsn() through the session's
// gateway (W7's catch-up target).
func (s *Session) ServerWALPosition(ctx context.Context) (uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	out := s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseStatement
		var v string
		if err := c.QueryRow(ctx, "select pg_current_wal_lsn()::text").Scan(&v); err != nil {
			return nil, err
		}
		return v, nil
	})
	if out.Type != OK {
		return 0, fmt.Errorf("pg_current_wal_lsn: %s %v", out.Attempt.Message, out.Err)
	}
	lsn, err := pglogrepl.ParseLSN(out.Value.(string))
	return uint64(lsn), err
}
