import json
import random
from datasets import load_dataset

# 选择较常用且体量适中的 BEIR 子集：scifact
dataset_name = "BeIR/scifact"

# 文档集合
corpus = load_dataset(dataset_name, "corpus", split="corpus")
corpus_map = {row["_id"]: row for row in corpus}

# 查询与标注（test）
queries = load_dataset(dataset_name, "queries", split="queries")
qrels = load_dataset("BeIR/scifact-qrels", split="test")

# qid -> [docid]
rel_map = {}
for r in qrels:
    if int(r.get("score", 0)) > 0:
        qid = str(r["query-id"])
        docid = str(r["corpus-id"])
        rel_map.setdefault(qid, []).append(docid)

# 取 200 条可评估 query（你可改）
all_q = [q for q in queries if str(q["_id"]) in rel_map]
random.seed(42)
random.shuffle(all_q)
selected = all_q[:200]

# 1) 生成你当前 Go 评估可用的 test_cases（query + relevant_docs）
test_cases = []
for q in selected:
    qid = q["_id"]
    rel_docs = [f"beir_{docid}.md" for docid in rel_map[qid]]
    test_cases.append({
        "query": q["text"],
        "relevant_docs": rel_docs
    })

# 2) 生成文档文件（写入 initfile）
# 你的系统按 fileName + 内容检索，这里把 BEIR 文档转成 md 文件名
import os
init_dir = "D:/RAG-Project/initfile"
os.makedirs(init_dir, exist_ok=True)

for q in selected:
    qid = q["_id"]
    for docid in rel_map[qid]:
        c = corpus_map.get(docid)
        if not c:
            continue
        fp = os.path.join(init_dir, f"beir_{docid}.md")
        if os.path.exists(fp):
            continue
        title = c.get("title") or ""
        text = c.get("text") or ""
        with open(fp, "w", encoding="utf-8") as f:
            f.write(f"# {title}\n\n{text}\n")

with open("D:/RAG-Project/test_cases_beir_scifact_200.json", "w", encoding="utf-8") as f:
    json.dump(test_cases, f, ensure_ascii=False, indent=2)

print(f"done: queries={len(test_cases)}")
print("output: D:/RAG-Project/test_cases_beir_scifact_200.json")