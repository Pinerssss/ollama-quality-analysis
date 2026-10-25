#!/usr/bin/env python3
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

COMMIT = "6383a0fa9cbf97494b847226e189f6e36b401a08"
SCRIPTS = Path(__file__).resolve().parent
FILES = ["server/routes.go", "server/sched.go", "llm/server.go",
         "middleware/openai.go", "manifest/layer.go"]
SPLIT = ["server/routes.go", "server/routes_generate.go", "server/routes_chat.go",
         "server/routes_blob.go", "server/routes_host.go"]

def portable_command(args, output, project, go):
    """Record commands with portable paths and executable names."""
    roots = sorted([(str(output), "<results>"), (str(project), "<project>")],
                   key=lambda item: len(item[0]), reverse=True)
    command = []
    for index, arg in enumerate(args):
        value = str(arg)
        if index == 0 and value == str(go):
            value = "go"
        elif index == 0 and value == sys.executable:
            value = "python3"
        for root, placeholder in roots:
            value = value.replace(root, placeholder)
        if "<results>" in value or "<project>" in value:
            value = value.replace(os.sep, "/")
        command.append(value)
    return command

def run(args, cwd, logfile=None, allow_failure=False):
    r = subprocess.run([str(x) for x in args], cwd=cwd, text=True,
                       env=dict(os.environ, GOTOOLCHAIN="local"),
                       stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if logfile:
        Path(logfile).write_text(r.stdout)
    if r.returncode and not allow_failure:
        raise RuntimeError(f"Command failed: {args}\n{r.stdout[-4000:]}")
    return r

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("repository", type=Path)
    p.add_argument("--output", required=True, type=Path)
    a = p.parse_args()
    repo = a.repository.resolve()
    out = a.output.resolve()
    if out.exists():
        raise ValueError("Use a new output directory to preserve earlier results")
    go = shutil.which("go")
    if not go:
        raise RuntimeError("Install Go 1.26.1 and add its bin directory to PATH")
    run(["git", "cat-file", "-e", COMMIT], repo)
    out.mkdir(parents=True)
    worktree = out / "prototype"
    run(["git", "worktree", "add", "--detach", worktree, COMMIT], repo)
    commands = []
    def record(args, log, allow=False):
        commands.append(portable_command(args, out, SCRIPTS.parent, go))
        return run(args, worktree, out / log, allow)
    try:
        suffix = ".exe" if os.name == "nt" else ""
        metrics = out / ("metrics" + suffix)
        record([go, "build", "-o", metrics, SCRIPTS / "metrics.go"], "build_metrics.log")
        record([metrics, worktree, *FILES], "baseline_metrics.json")
        record([go, "test", "-count=1", "-coverprofile=" + str(out / "baseline_coverage.out"),
                "./middleware", "./openai"], "baseline_test.log")
        record([go, "tool", "cover", "-func=" + str(out / "baseline_coverage.out")],
               "baseline_coverage_functions.txt")
        baseline_server = record([go, "test", "-count=1", "./server"],
                                 "baseline_server_test.log", True)
        # save the original route declarations for an independent structural check.
        reference = out / "reference" / "server"
        reference.mkdir(parents=True)
        shutil.copy2(worktree / "server/routes.go", reference / "routes.go")
        patch = SCRIPTS.parent / "improvements.patch"
        record(["git", "apply", "--check", patch], "patch_check.log")
        record(["git", "apply", patch], "patch_apply.log")
        record([metrics, worktree, *SPLIT], "after_metrics.json")
        verify = out / ("verify_split" + suffix)
        record([go, "build", "-o", verify, SCRIPTS / "verify_split.go"], "build_verify.log")
        record([verify, out / "reference", worktree], "structural_verification.json")
        record([go, "test", "-count=1", "-coverprofile=" + str(out / "after_coverage.out"),
                "./middleware", "./openai"], "after_test.log")
        record([go, "tool", "cover", "-func=" + str(out / "after_coverage.out")],
               "after_coverage_functions.txt")
        record([go, "test", "-count=1", "-run", "^$", "./server"], "server_compile.log")
        after_server = record([go, "test", "-count=1", "./server"], "server_test.log", True)
        record([go, "test", "-count=1", "-run",
                "Test(Chat|Generate|Embed|HeadBlob|OptionsForPrompt|Scheduler|Sched|Allowed)",
                "./server"], "targeted_server_test.log")
        # compile the actual changed helper body against Ollama's actual api.Options.
        demo = out / "helper"
        demo.mkdir()
        text = (worktree / "server/routes.go").read_text()
        helper = text[text.index("type contextLengthProvider interface"):
                      text.index("type chatExecutionMode int")]
        (demo / "options_for_prompt.go").write_text(
            'package server\nimport "github.com/ollama/ollama/api"\n' + helper)
        shutil.copy2(SCRIPTS / "options_for_prompt_test.go.txt", demo / "options_for_prompt_test.go")
        record([go, "test", "-count=1", "-v", demo / "options_for_prompt.go",
                demo / "options_for_prompt_test.go"], "helper_test.log")
        for stage in ["baseline", "after"]:
            record([sys.executable, SCRIPTS / "coverage_summary.py", out / (stage + "_coverage.out")],
                   stage + "_coverage.json")
        (out / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
        (out / "run_status.json").write_text(json.dumps({
            "baseline_commit": COMMIT,
            "go_version": run([go, "version"], SCRIPTS).stdout.strip(),
            "baseline_full_server_exit": baseline_server.returncode,
            "after_full_server_exit": after_server.returncode,
            "note": "Full-server failures are retained, never silently excluded. Inspect both logs."
        }, indent=2) + "\n")
        print("Completed. Results:", out)
        if baseline_server.returncode or after_server.returncode:
            print("Full server tests have failures; inspect baseline_server_test.log and server_test.log.")
    finally:
        run(["git", "worktree", "remove", "--force", worktree], repo, allow_failure=True)

if __name__ == "__main__":
    main()
