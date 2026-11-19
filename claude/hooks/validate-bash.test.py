#!/usr/bin/env python3
"""Regression test for validate-bash.sh push-wrapper detection.
Run: python3 -I ~/.claude/hooks/validate-bash.test.py [hook-path]. Exit 1 on any failure."""
import json, os, subprocess, sys, tempfile

hook = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(os.path.abspath(__file__)), "validate-bash.sh")
P = "git" + " push"  # built at runtime so this file never trips the hook it tests
d = tempfile.mkdtemp()
fx = {
    "inner.sh": f'#!/usr/bin/env bash\ncd "$1" && {P} --force-with-lease origin main\n',
    "wrapper.sh": f'#!/usr/bin/env bash\nexec {d}/inner.sh "$@"\n',
    "clean.sh": "#!/usr/bin/env bash\ngit status\necho ok\n",
    "commented.sh": f"#!/usr/bin/env bash\n# never run {P} here\necho ok\n",
}
for name, body in fx.items():
    p = os.path.join(d, name)
    with open(p, "w") as fh:
        fh.write(body)
    os.chmod(p, 0o755)

cases = [
    ("block", f"{d}/wrapper.sh"),
    ("block", f"bash {d}/inner.sh /tmp"),
    ("block", f"cd {d} && ./wrapper.sh"),
    ("block", f"FOO=1 {d}/wrapper.sh x"),
    ("block", f"echo hi; source {d}/inner.sh"),
    ("block", f"exec {d}/wrapper.sh"),
    ("block", f"{P} origin main"),
    ("allow", f"cat {d}/inner.sh"),
    ("allow", f"rg push {d}/inner.sh"),
    ("allow", f"{d}/clean.sh"),
    ("allow", f"{d}/commented.sh"),
    ("allow", "git status && ls -la"),
    ("allow", f"bash -n {d}/inner.sh"),
    ("allow", f"{d}/missing.sh"),
]
failed = 0
for want, cmd in cases:
    ev = json.dumps({"tool_name": "Bash", "tool_input": {"command": cmd}, "cwd": d})
    got = "block" if subprocess.run(["bash", hook], input=ev, text=True, capture_output=True).returncode == 2 else "allow"
    if got != want:
        failed += 1
        print(f"FAIL want={want} got={got}: {cmd}")
for name in fx:
    os.remove(os.path.join(d, name))
os.rmdir(d)
print(f"passed {len(cases) - failed}   failed {failed}   total {len(cases)}")
sys.exit(1 if failed else 0)
