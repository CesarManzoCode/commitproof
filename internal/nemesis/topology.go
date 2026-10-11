// Package nemesis injects, confirms and heals the faults of CONTRACT §8 from
// the runner container (ADR-0018): every action is an external command —
// `docker exec` into the nem-* helper of a network namespace or into a node
// container, `docker` lifecycle commands, or the runner's own status probe —
// recorded in the run's commands.jsonl. Confirmation and heal predicates
// parse the raw outputs of probe commands, never only an exit code, and the
// raw outputs are copied into the fault event's evidence (§15 K3.4). A
// predicate that does not hold within its deadline ends the fault as
// not-applied / not-healed and the run as INVALID (fail closed).
//
// The package is linked only into commitproof; verdict binaries never import
// it (§5.3).
package nemesis

import (
	"fmt"
	"sort"
)

const project = "commitproof"

// Container is the Compose container name of a service.
func Container(svc string) string { return project + "-" + svc + "-1" }

// RunnerContainer is the name the host gives the runner container of a run;
// NemRunner is the helper the runner starts in its own namespace.
const (
	RunnerContainer = project + "-runner"
	NemRunner       = project + "-nem-runner"
)

// RunnerProbePort is where the runner accepts TCP probes during a run, so
// that peers can probe the runner's namespace (F1 "every other cpnet
// address").
const RunnerProbePort = 7999

// NS is one network namespace of the topology (CONTRACT §5.1): its name, the
// last octet of its cpnet address, a port that accepts connections whenever
// the namespace's main process runs (used by connect probes), and the
// container that owns it.
type NS struct {
	Name  string
	Octet int
	Port  int
	Owner string // service owning the namespace
}

// Namespaces of the topology. Nodes are named n1…n4: pg-nX owns the
// namespace and pooler-nX joins it; the probe port is pgctld's gRPC port
// (pgctld is the node container's main process, up even while PostgreSQL or
// multipooler is down).
var Namespaces = []NS{
	{"etcd", 10, 2379, "etcd"},
	{"n1", 11, 16200, "pg-n1"}, {"n2", 12, 16200, "pg-n2"}, {"n3", 13, 16200, "pg-n3"}, {"n4", 14, 16200, "pg-n4"},
	{"orch-z1", 21, 17000, "orch-z1"}, {"orch-z2", 22, 17000, "orch-z2"}, {"orch-z3", 23, 17000, "orch-z3"},
	{"gw-z1", 31, 15432, "gw-z1"}, {"gw-z2", 32, 15432, "gw-z2"}, {"gw-z3", 33, 15432, "gw-z3"},
	{"runner", 40, RunnerProbePort, "runner"},
}

// Nodes are the pooler nodes.
var Nodes = []string{"n1", "n2", "n3", "n4"}

// Cells of the nodes (§5.1); n4 is in zone1.
var nodeCell = map[string]string{"n1": "zone1", "n2": "zone2", "n3": "zone3", "n4": "zone1"}

// AppName is a node's replication application_name ("{cell}_{name}", the
// form multipooler uses for synchronous_standby_names).
func AppName(node string) string { return nodeCell[node] + "_" + node }

// LookupNS returns the namespace named n.
func LookupNS(n string) (NS, bool) {
	for _, ns := range Namespaces {
		if ns.Name == n {
			return ns, true
		}
	}
	return NS{}, false
}

// IsNode reports whether n names a pooler node.
func IsNode(n string) bool {
	for _, x := range Nodes {
		if x == n {
			return true
		}
	}
	return false
}

// OwnerContainer is the container that owns namespace n.
func OwnerContainer(n string) string {
	if n == "runner" {
		return RunnerContainer
	}
	ns, _ := LookupNS(n)
	return Container(ns.Owner)
}

// Helper is the nem-* container of namespace n.
func Helper(n string) string {
	if n == "runner" {
		return NemRunner
	}
	ns, _ := LookupNS(n)
	return Container("nem-" + ns.Owner)
}

// Addr is the cpnet address of namespace n under prefix ("10.77.0").
func Addr(prefix, n string) string {
	ns, _ := LookupNS(n)
	return fmt.Sprintf("%s.%d", prefix, ns.Octet)
}

// ProbeAddr is Addr with the namespace's probe port.
func ProbeAddr(prefix, n string) string {
	ns, _ := LookupNS(n)
	return fmt.Sprintf("%s.%d:%d", prefix, ns.Octet, ns.Port)
}

// RunningNamespaces lists the namespaces whose owner container is running,
// given the set of running container names, sorted.
func RunningNamespaces(running map[string]bool) []string {
	var out []string
	for _, ns := range Namespaces {
		if running[OwnerContainer(ns.Name)] {
			out = append(out, ns.Name)
		}
	}
	sort.Strings(out)
	return out
}

func without(xs []string, drop ...string) []string {
	d := map[string]bool{}
	for _, x := range drop {
		d[x] = true
	}
	var out []string
	for _, x := range xs {
		if !d[x] {
			out = append(out, x)
		}
	}
	return out
}
