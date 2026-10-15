package report

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
)

// Number formatting from integers only (§15 K6 determinism rules).

// secs renders ns as seconds with three decimals, rounded half up.
func secs(ns int64) string {
	neg := ns < 0
	if neg {
		ns = -ns
	}
	ms := (ns + 500_000) / 1_000_000
	s := fmt.Sprintf("%d.%03d", ms/1000, ms%1000)
	if neg {
		return "−" + s
	}
	return s
}

// msec renders ns as milliseconds with one decimal, rounded half up.
func msec(ns int64) string {
	t := (ns + 50_000) / 100_000
	return fmt.Sprintf("%d.%d", t/10, t%10)
}

func esc(s string) string { return html.EscapeString(s) }

// clip shortens s to n runes (deterministically) for labels.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// dataBlock encodes v as canonical JSON safe inside a <script> element.
func dataBlock(v any) ([]byte, error) {
	b, err := canonjson.Marshal(v)
	if err != nil {
		return nil, err
	}
	b = bytes.ReplaceAll(b, []byte("&"), []byte(`&`))
	b = bytes.ReplaceAll(b, []byte("<"), []byte(`<`))
	b = bytes.ReplaceAll(b, []byte(">"), []byte(`>`))
	return b, nil
}

// canonText renders a JSON value canonically for display.
func canonText(v any) string {
	b, err := canonjson.Marshal(v)
	if err != nil {
		return "(" + err.Error() + ")"
	}
	return string(b)
}

func joinInts(xs []int64) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, " ")
}

// css is the page style (inline; no external asset).
const css = `
:root{--ok:#2a9d4b;--fail:#d62828;--info:#e79a00;--ink:#1d2433;--mute:#5c6577;--line:#d9dde5;--bg:#ffffff;--panel:#f5f7fa;--hl:#0b57d0}
*{box-sizing:border-box}
body{margin:0;padding:16px;font:14px/1.45 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;color:var(--ink);background:var(--bg)}
h1{font-size:20px;margin:0 0 4px}h2{font-size:16px;margin:28px 0 8px;border-bottom:1px solid var(--line);padding-bottom:4px}
h3{font-size:14px;margin:16px 0 6px}
p{margin:6px 0}code,pre,.mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px}
pre{white-space:pre-wrap;word-break:break-all;background:var(--panel);padding:8px;border-radius:4px;max-height:320px;overflow:auto}
nav a{margin-right:12px}
table{border-collapse:collapse;margin:6px 0 10px;font-size:13px}
th,td{border:1px solid var(--line);padding:3px 7px;text-align:left;vertical-align:top}
th{background:var(--panel);font-weight:600}td.n{text-align:right;font-variant-numeric:tabular-nums}
.verdict{display:inline-block;padding:1px 8px;border-radius:3px;font-weight:600;color:#fff;background:var(--mute)}
.v-PASS{background:var(--ok)}.v-FAIL{background:var(--fail)}.v-UNKNOWN,.v-INVALID{background:var(--info)}
.legend span{display:inline-block;margin-right:14px}.sw{display:inline-block;width:12px;height:10px;margin-right:4px;vertical-align:-1px}
.scroll{overflow-x:auto;border:1px solid var(--line);border-radius:4px;background:#fff}
svg text{font:11px system-ui,sans-serif;fill:var(--ink)}
svg .lbl{fill:var(--mute)}
.ok{fill:var(--ok)}.fail{fill:var(--fail)}.info{fill:var(--info)}
.mk{stroke:#8a93a6;stroke-width:1;stroke-dasharray:2 3}
.fb{fill:#b9c4d6;fill-opacity:.25}.f-a{fill:#e9b44c}.f-b{fill:#c0392b}.f-c{fill:#6c8ebf}
.ph{fill:#1d2433}.ph-bad{fill:var(--fail)}
.st-primary{fill:#1f4e9c}.st-standby{fill:#9bb7e0}.st-error{fill:var(--fail)}.st-down{fill:#555}
.sl-ready{fill:#3b8f6a}.sl-not{fill:#d9a400}
.cobs{fill:#7d4ea8}
.grid{stroke:#eef0f4;stroke-width:1}
.axis{stroke:#9aa3b5;stroke-width:1}
.series{fill:none;stroke-width:1.5}
.bar{fill:var(--fail);fill-opacity:.6}
:target{stroke:var(--hl);stroke-width:3;outline:2px solid var(--hl)}
li:target,tr:target{background:#e8f0fe}
.anoms li{margin:4px 0}
.small{font-size:12px;color:var(--mute)}
`
