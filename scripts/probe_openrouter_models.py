#!/usr/bin/env python3
"""Probe OpenRouter models for what an agent needs: tool calls and their round trip.

Usage (PowerShell):
    $env:OPENROUTER_API_KEY = "sk-or-..."          # or the script asks for it, hidden
    python scripts/probe_openrouter_models.py            # the free candidates
    python scripts/probe_openrouter_models.py --paid     # also the cheap paid open models
    python scripts/probe_openrouter_models.py MODEL_ID [MODEL_ID ...]
    python scripts/probe_openrouter_models.py --env-file private/.env.dev   # read the key from a file

The key is read from OPENROUTER_API_KEY, from the OPENROUTER_API_KEY line of an
--env-file (no other line is read), or typed hidden, and only sent to
openrouter.ai; it is never printed or written. Results go to the screen and to
probe_results.json (no key in it). Python 3, standard library only.

Four small checks per model, one request each (free models are rate limited, so
the script waits between calls):
  math       "17 * 23" must give 391 (is the model awake)
  tool       must answer with a get_time(city) tool call and valid JSON arguments
  roundtrip  given the tool result, must use it in its answer (multi-turn tools)
  french     answers in French when asked
"""
import getpass
import json
import os
import sys
import time
import urllib.error
import urllib.request

URL = "https://openrouter.ai/api/v1/chat/completions"

FREE = [
    "nvidia/nemotron-3-ultra-550b-a55b:free",
    "nvidia/nemotron-3-super-120b-a12b:free",
    "nvidia/nemotron-3.5-lightning:free",
    "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free",
    "thinkingmachines/inkling:free",
    "thinkingmachines/inkling-small:free",
    "cohere/north-mini-code:free",
    "google/gemma-4-31b-it:free",
    "google/gemma-4-26b-a4b-it:free",
    "poolside/laguna-s-2.1:free",  # going away October 31, 2026
    "dots-studio/dots-3-note-preview:free",  # going away December 31, 2026
    "liquid/lfm-2.5-2.6b:free",  # the vendor advises against agentic use
    "openrouter/free",
]
PAID = [
    "openai/gpt-oss-120b",
    "openai/gpt-oss-20b",
    "deepseek/deepseek-v4.1-flash",
    "qwen/qwen3.8-flash",
    "z-ai/glm-5.3-flash",
    "moonshotai/kimi-k2.7-code",
    "mistralai/devstral-2512",
    "minimax/minimax-m3",
]

TOOLS = [{
    "type": "function",
    "function": {
        "name": "get_time",
        "description": "Get the current local time in a city.",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string", "description": "City name"}},
            "required": ["city"],
        },
    },
}]


def call(key, model, messages, tools=None, max_tokens=1500):
    body = {"model": model, "messages": messages, "max_tokens": max_tokens, "temperature": 0}
    if tools:
        body["tools"] = tools
        body["tool_choice"] = "auto"
    req = urllib.request.Request(
        URL,
        data=json.dumps(body).encode(),
        headers={"Authorization": "Bearer " + key, "Content-Type": "application/json"},
    )
    started = time.time()
    for attempt in (1, 2):
        try:
            with urllib.request.urlopen(req, timeout=120) as resp:
                data = json.load(resp)
            return data, time.time() - started, None
        except urllib.error.HTTPError as err:
            text = err.read().decode("utf-8", "replace")[:200]
            if err.code == 429 and attempt == 1:
                time.sleep(25)  # free rate limit: wait once and retry
                continue
            return None, time.time() - started, f"HTTP {err.code}: {text}"
        except Exception as err:  # network, timeout
            return None, time.time() - started, str(err)[:200]
    return None, time.time() - started, "unreachable"


def message_of(data):
    try:
        return data["choices"][0]["message"]
    except (KeyError, IndexError, TypeError):
        return {}


def probe(key, model):
    result = {"model": model, "notes": []}
    times = []

    def run(name, messages, tools=None, check=None):
        data, took, err = call(key, model, messages, tools)
        times.append(took)
        if err or not data:
            result[name] = False
            result["notes"].append(f"{name}: {err or 'empty'}")
            return None
        msg = message_of(data)
        ok = bool(check(msg))
        result[name] = ok
        if not ok:
            snippet = (msg.get("content") or "").strip().replace("\n", " ")[:80]
            result["notes"].append(f"{name}: {snippet or 'no content'}")
        time.sleep(3)
        return msg

    run("math", [{"role": "user", "content": "What is 17 * 23? Answer with just the number."}],
        check=lambda m: "391" in (m.get("content") or ""))

    def has_tool_call(m):
        calls = m.get("tool_calls") or []
        if not calls or calls[0].get("function", {}).get("name") != "get_time":
            return False
        try:
            return "city" in json.loads(calls[0]["function"].get("arguments") or "{}")
        except ValueError:
            return False

    ask = [{"role": "user", "content": "What time is it in Paris right now? Use the tool."}]
    msg = run("tool", ask, tools=TOOLS, check=has_tool_call)

    if msg and has_tool_call(msg):
        call0 = msg["tool_calls"][0]
        follow = ask + [
            {"role": "assistant", "content": msg.get("content") or "", "tool_calls": [call0]},
            {"role": "tool", "tool_call_id": call0.get("id", "call_1"), "content": json.dumps({"time": "14:35"})},
        ]
        run("roundtrip", follow, tools=TOOLS, check=lambda m: "14:35" in (m.get("content") or "") or "14h35" in (m.get("content") or ""))
    else:
        result["roundtrip"] = False
        result["notes"].append("roundtrip: skipped (no tool call)")

    run("french", [{"role": "user", "content": "Réponds en français, en une phrase courte: pourquoi le ciel est-il bleu?"}],
        check=lambda m: "ciel" in (m.get("content") or "").lower() or "lumière" in (m.get("content") or "").lower())

    result["avg_seconds"] = round(sum(times) / max(len(times), 1), 1)
    return result


def key_from_env_file(path):
    """Return the OPENROUTER_API_KEY value of a KEY=VALUE file, reading no other line."""
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line.startswith("OPENROUTER_API_KEY="):
                return line.split("=", 1)[1].strip().strip('"').strip("'")
    return ""


def main():
    argv = sys.argv[1:]
    env_file = None
    if "--env-file" in argv:
        i = argv.index("--env-file")
        env_file = argv[i + 1]
        del argv[i:i + 2]
    args = [a for a in argv if not a.startswith("--")]
    models = args or (FREE + (PAID if "--paid" in argv else []))
    key = (key_from_env_file(env_file) if env_file else "") or os.environ.get("OPENROUTER_API_KEY") or getpass.getpass("OpenRouter API key (hidden): ")
    if not key.strip():
        sys.exit("no key")
    rows = []
    print(f"{'model':54} math tool trip fr  avg(s)")
    for model in models:
        row = probe(key.strip(), model)
        rows.append(row)
        mark = lambda b: "ok  " if b else "FAIL"
        print(f"{model:54} {mark(row['math'])} {mark(row['tool'])} {mark(row['roundtrip'])} {mark(row['french'])} {row['avg_seconds']}")
        for note in row["notes"]:
            print("      -", note)
    with open("probe_results.json", "w", encoding="utf-8") as f:
        json.dump(rows, f, indent=2, ensure_ascii=False)
    print("\nWrote probe_results.json (no key in it).")


if __name__ == "__main__":
    main()
