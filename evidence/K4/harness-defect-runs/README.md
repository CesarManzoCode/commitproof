# Runs set aside by a harness defect (CONTRACT §11.1, §12.1)

These three sealed runs are the K4 attempts of matrix cell C4 / baseline /
real / seed 2 made with commitproof `f4a5a0f` (record `campaign-k4-1`). Each
ended INVALID with `fault f1.0 (F1a) not-healed: heal predicate did not hold
within 30s`: in C4's "F5b during F1a(leader)" (ADR-0026) F5b stops n4 while
F1a holds, and F1a's heal predicate still required connects to and from n4
and `iptables-save` in n4's stopped helper (`problems: n3->n4 did not
connect, n4->n3 did not connect, n4: iptables-save failed`). A HARNESS_DEFECT
(§11.1): fixed in `f84bf4f` (F1 heals probe only the namespaces that still
exist and list the vanished ones), and the cell re-run with the fixed build
(records `build-2`, `images-build-2`, `campaign-k4-2`).

They are archived here, outside `runs/`, because the campaign driver counts
every sealed run of a cell's plan as an attempt and stops at three; the
re-run uses the same plan. Their checks.json (cpcheck: INVALID, reason
not-healed) are kept next to them. Nothing in them was changed: each archive
is the sealed run directory (zstd tar, entries sorted, owner 0, mtime 0) and
`cpcheck verify-seal` passes on the extracted directory.

The nine INVALID C5 attempts (F2a not-healed under the original C5 heal
order, ADR-0028) stay in `runs/`: C5's plans changed with the fix, so the
re-runs are different plans and the old attempts do not block them.
