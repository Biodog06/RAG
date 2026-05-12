import os, sys

target = "ragas_eval.py"
if os.path.exists(target):
    with open(target, "r", encoding="utf-8", errors="replace") as f:
        content = f.read()
    content = content.replace("sk-153c477a54e54a9ba32f216b0bc4e8ac", "REDACTED")
    content = content.replace("sk-6f83184867514c0fb4ead4d31ec11303", "REDACTED")
    with open(target, "w", encoding="utf-8", newline="\n") as f:
        f.write(content)
