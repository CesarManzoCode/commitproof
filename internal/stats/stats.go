// Package stats computes the campaign statistics of CONTRACT §12.3 with
// integer arithmetic only (§15 K6 determinism rules, ADR-0036): the Wilson
// 95 % interval of k/N and the one-sided 95 % zero-failure bound
// 1 − 0.05^(1/N), at a 10⁻¹² fixed point (math/big), rendered with three
// decimals — lower ends rounded down, upper ends rounded up, so a rendered
// bound is never tighter than the exact one.
package stats

import (
	"fmt"
	"math/big"
)

// Scale is the fixed point: a Fixed value v means v / Scale.
const Scale int64 = 1_000_000_000_000

// Fixed is a probability in units of 1/Scale, in [0, Scale].
type Fixed int64

// Interval is a closed interval of probabilities.
type Interval struct{ Lo, Hi Fixed }

// z = 1.96 = 196/100 and z² = 3.8416 = 38416/10⁴ (95 %, two-sided).
const (
	z100   = 196
	z2e4   = 38416
	tenK   = 10_000
	fortyK = 40_000
)

func bi(v int64) *big.Int { return big.NewInt(v) }

func ceilDiv(a, b *big.Int) *big.Int {
	q, r := new(big.Int).QuoRem(a, b, new(big.Int))
	if r.Sign() > 0 {
		q.Add(q, bi(1))
	}
	return q
}

func ceilSqrt(x *big.Int) *big.Int {
	s := new(big.Int).Sqrt(x)
	if new(big.Int).Mul(s, s).Cmp(x) < 0 {
		s.Add(s, bi(1))
	}
	return s
}

func clamp(v *big.Int) Fixed {
	switch {
	case v.Sign() < 0:
		return 0
	case v.Cmp(bi(Scale)) > 0:
		return Fixed(Scale)
	}
	return Fixed(v.Int64())
}

// Wilson95 returns the Wilson score interval (z = 1.96) of k successes in n
// trials: center c = (k + z²/2)/(n + z²), half-width h = z·√(k(n−k)/n +
// z²/4)/(n + z²); [⌊c⌋ − ⌈h⌉, ⌈c⌉ + ⌈h⌉] at the fixed point, clamped to
// [0, 1].
func Wilson95(k, n int64) (Interval, error) {
	if n <= 0 || k < 0 || k > n {
		return Interval{}, fmt.Errorf("stats: Wilson interval of %d/%d", k, n)
	}
	S := bi(Scale)
	d := bi(n*tenK + z2e4)    // (n + z²)·10⁴
	cn := bi(k*tenK + z2e4/2) // (k + z²/2)·10⁴
	cS := new(big.Int).Mul(cn, S)
	cLo := new(big.Int).Quo(cS, d)
	cHi := ceilDiv(cS, d)
	// z/(n + z²) = 19600/d, so h² = 19600²·(k(n−k)/n + z²/4)/d² =
	// 196²·10⁴·A/(B·d²) with A = 40000·k(n−k) + 38416·n, B = 40000·n.
	a := new(big.Int).Add(new(big.Int).Mul(bi(fortyK), bi(k*(n-k))), bi(z2e4*n))
	num := new(big.Int).Mul(a, bi(z100*z100*tenK))
	num.Mul(num, new(big.Int).Mul(S, S))
	den := new(big.Int).Mul(bi(fortyK*n), new(big.Int).Mul(d, d))
	h := ceilSqrt(ceilDiv(num, den))
	return Interval{Lo: clamp(new(big.Int).Sub(cLo, h)), Hi: clamp(new(big.Int).Add(cHi, h))}, nil
}

// ZeroFailureBound95 returns the one-sided 95 % upper bound on the per-run
// probability after n runs without a failure, 1 − 0.05^(1/n): Scale − r for
// the greatest r with 20·r^n ≤ Scale^n (r ≤ Scale·0.05^(1/n), so the bound
// is not below the exact one).
func ZeroFailureBound95(n int64) (Fixed, error) {
	if n <= 0 {
		return 0, fmt.Errorf("stats: zero-failure bound of %d runs", n)
	}
	limit := new(big.Int).Exp(bi(Scale), bi(n), nil)
	ok := func(r int64) bool {
		p := new(big.Int).Exp(bi(r), bi(n), nil)
		return p.Mul(p, bi(20)).Cmp(limit) <= 0
	}
	lo, hi := int64(0), Scale // ok(lo); the answer is in [lo, hi]
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		if ok(mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return Fixed(Scale - lo), nil
}

const milli = Scale / 1000

// Down renders v with three decimals, rounded down.
func Down(v Fixed) string { return three(int64(v) / milli) }

// Up renders v with three decimals, rounded up.
func Up(v Fixed) string { return three((int64(v) + milli - 1) / milli) }

func three(m int64) string { return fmt.Sprintf("%d.%03d", m/1000, m%1000) }
