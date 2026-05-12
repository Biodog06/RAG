"""
RAGAS 评估脚本
使用 RAGAS 框架对 RAG 系统进行端到端评估。

指标：
  - faithfulness       : 答案是否忠实于检索到的上下文
  - answer_relevancy   : 答案是否与问题相关
  - context_precision  : 检索上下文中相关内容的精确率
  - context_recall     : 检索上下文对 ground truth 的召回率

运行前请确保 Go 服务器已启动（默认 :8081）。
"""

import sys
import io
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')
sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8', errors='replace')

import json
import os
import requests
import time
from pathlib import Path
from openai import OpenAI

# ── 配置 ──────────────────────────────────────────────────────────────────────
SERVER_BASE   = "http://localhost:8081/api/v1"
LOGIN_URL     = f"{SERVER_BASE}/users/login"
SEARCH_URL    = f"{SERVER_BASE}/search/hybrid"

DEEPSEEK_KEY  = os.environ.get("DEEPSEEK_API_KEY", "")
DEEPSEEK_BASE = "https://api.deepseek.com/v1"
DEEPSEEK_MODEL= "deepseek-chat"

INITFILE_DIR  = Path("initfile")
TEST_CASES    = Path(os.getenv("TEST_CASES_FILE", "test_cases_crypto.json"))
OUTPUT_JSON   = Path(os.getenv("OUTPUT_FILE", "ragas_samples.json"))

TOP_K = 5   # 每次检索返回的 chunk 数

# ── 登录获取 JWT ───────────────────────────────────────────────────────────────
def get_token() -> str:
    resp = requests.post(LOGIN_URL, json={"username": "admin", "password": "admin123"}, timeout=10)
    resp.raise_for_status()
    data = resp.json()
    token = (data.get("data") or {}).get("token") or data.get("token")
    if not token:
        raise RuntimeError(f"登录失败，响应: {data}")
    print(f"[Auth] 登录成功，token 前缀: {token[:20]}...")
    return token

# ── 调用搜索 API ───────────────────────────────────────────────────────────────
def search(query: str, token: str) -> list[dict]:
    resp = requests.get(
        SEARCH_URL,
        params={"query": query, "topK": TOP_K},
        headers={"Authorization": f"Bearer {token}"},
        timeout=60,
    )
    resp.raise_for_status()
    return resp.json().get("data", [])

# ── 用 DeepSeek 生成答案 ───────────────────────────────────────────────────────
def generate_answer(query: str, contexts: list[str], client: OpenAI) -> str:
    ctx_text = "\n\n".join(f"[{i+1}] {c}" for i, c in enumerate(contexts))
    prompt = (
        f"请根据以下参考资料回答问题，仅使用资料中的信息，不要编造。\n\n"
        f"参考资料：\n{ctx_text}\n\n"
        f"问题：{query}\n\n答案："
    )
    resp = client.chat.completions.create(
        model=DEEPSEEK_MODEL,
        messages=[{"role": "user", "content": prompt}],
        temperature=0.1,
        max_tokens=800,
    )
    return resp.choices[0].message.content.strip()

# ── 读取 ground truth（相关文档内容拼接）────────────────────────────────────────
def load_ground_truth(relevant_docs: list[str]) -> str:
    parts = []
    for doc in relevant_docs:
        p = INITFILE_DIR / doc
        if p.exists():
            parts.append(p.read_text(encoding="utf-8")[:2000])
    return "\n\n".join(parts) if parts else ""

# ── RAGAS 评估（可选，当前流程不调用）────────────────────────────────────────────
def run_ragas(samples: list[dict]):
    from datasets import Dataset
    from ragas import evaluate
    from ragas.metrics.collections import (
        faithfulness,
        answer_relevancy,
        context_precision,
        context_recall,
    )
    from langchain_openai import ChatOpenAI, OpenAIEmbeddings

    llm = ChatOpenAI(
        model=DEEPSEEK_MODEL,
        openai_api_key=DEEPSEEK_KEY,
        openai_api_base=DEEPSEEK_BASE,
        temperature=0,
        n=1,
    )
    embeddings = OpenAIEmbeddings(
        model="text-embedding-v4",
        openai_api_key=os.environ.get("DASHSCOPE_API_KEY", ""),
        openai_api_base="https://dashscope.aliyuncs.com/compatible-mode/v1",
    )

    dataset = Dataset.from_list(samples)
    return evaluate(
        dataset,
        metrics=[faithfulness, answer_relevancy, context_precision, context_recall],
        llm=llm,
        embeddings=embeddings,
        raise_exceptions=False,
    )

# ── 主流程 ─────────────────────────────────────────────────────────────────────
def main():
    # 1. 加载测试用例
    cases = json.loads(TEST_CASES.read_text(encoding="utf-8"))
    print(f"[Info] 加载 {len(cases)} 条测试用例")

    # 2. 登录
    try:
        token = get_token()
    except Exception as e:
        print(f"[Error] 无法登录服务器: {e}")
        print("请先启动 Go 服务器: go run cmd/server/main.go")
        sys.exit(1)

    # 3. DeepSeek 客户端
    llm_client = OpenAI(api_key=DEEPSEEK_KEY, base_url=DEEPSEEK_BASE)

    # 4. 逐条构建评估样本
    samples = []
    for i, case in enumerate(cases):
        query = case["query"]
        print(f"[{i+1}/{len(cases)}] 处理: {query[:40]}")

        # 检索
        try:
            hits = search(query, token)
        except Exception as e:
            print(f"  ⚠ 检索失败: {e}")
            continue

        contexts = [h["textContent"] for h in hits if h.get("textContent")]
        if not contexts:
            print("  ⚠ 无检索结果，跳过")
            continue

        # 生成答案
        try:
            answer = generate_answer(query, contexts, llm_client)
        except Exception as e:
            print(f"  ⚠ 生成答案失败: {e}")
            continue

        # ground truth
        ground_truth = load_ground_truth(case.get("relevant_docs", []))

        samples.append({
            "question":     query,
            "answer":       answer,
            "contexts":     contexts,
            "ground_truth": ground_truth,
        })
        print(f"  OK contexts={len(contexts)}, answer_len={len(answer)}")
        time.sleep(0.3)  # 避免 API 限速

    print(f"\n[Info] 共构建 {len(samples)} 条有效样本")

    # 5. 仅写样本，不执行 RAGAS evaluate
    sample_pack = {
        "generated_at": time.strftime("%Y-%m-%d %H:%M:%S"),
        "count": len(samples),
        "samples": samples,
    }
    OUTPUT_JSON.write_text(json.dumps(sample_pack, ensure_ascii=False, indent=2), encoding="utf-8")

    print("\n" + "═" * 60)
    print("  样本生成完成（未执行 RAGAS 打分）")
    print("═" * 60)
    print(f"  样本数: {len(samples)}")
    print(f"  输出文件: {OUTPUT_JSON}")
    print("═" * 60)


if __name__ == "__main__":
    main()
