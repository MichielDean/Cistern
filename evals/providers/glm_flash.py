"""Custom promptfoo provider: glm-5.3-flash:cloud via Ollama /v1, reasoning_effort=low.

Non-streaming call, returns only `content` (reasoning stays in the separate
field — promptfoo's built-in providers concatenate reasoning deltas into the
output, which pollutes assertions). Retries on empty/error responses: ollama
under concurrency occasionally returns truncated or empty completions.
"""
import json
import os
import time
import urllib.request


def call_api(prompt, options, context):
    config = options.get('config', {}) or {}
    api_base = config.get('apiBaseUrl', 'http://localhost:11434/v1')
    body = {
        'model': 'glm-5.3-flash:cloud',
        'messages': [{'role': 'user', 'content': prompt}],
        'temperature': config.get('temperature', 0.1),
        'max_tokens': config.get('max_tokens', 8000),
        'stream': False,
        'reasoning_effort': config.get('reasoning_effort', 'low'),
    }
    last_err = None
    for attempt in range(3):
        req = urllib.request.Request(
            f'{api_base}/chat/completions',
            data=json.dumps(body).encode(),
            headers={'Content-Type': 'application/json'},
        )
        try:
            r = json.load(urllib.request.urlopen(req, timeout=420))
            choice = r['choices'][0]
            content = choice['message'].get('content') or ''
            usage = r.get('usage') or {}
            dbg = os.environ.get('GLM_FLASH_DEBUG_LOG')
            if dbg:
                with open(dbg, 'a') as f:
                    f.write(json.dumps({'prompt_head': str(prompt)[:200], 'content': content[:1500], 'tokens': usage}) + '\n')
            if content.strip():
                return {
                    'output': content,
                    'tokenUsage': {
                        'prompt': usage.get('prompt_tokens', 0),
                        'completion': usage.get('completion_tokens', 0),
                        'total': usage.get('total_tokens', 0),
                    },
                }
            last_err = 'empty content from provider'
        except Exception as e:
            last_err = str(e)
        time.sleep(2 * (attempt + 1))
    return {'error': last_err or 'no content', 'output': ''}
