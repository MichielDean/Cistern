"""Custom promptfoo grader provider for the cataractae eval suite.

Judge model, configured per-config in YAML (defaultTest.options.provider.config.model).
Defaults to beam:cloud — grader matrix (2026-11-13, subject glm-5.3-flash fixed):
best rubric alignment across 10 configs (2 fails/64 vs flash 3, minimax 4,
kimi-k3.1-pro 6), non-GLM family avoids subject self-bias, 8x cheaper than
kimi-k3.1-pro. Residual single-shot flake churn is inherent and absorbed by the
gate's <=2-flake tolerance. Override per-config in YAML (model:).
"""
import json
import time
import urllib.request


def call_api(prompt, options, context):
    config = options.get('config', {}) or {}
    api_base = config.get('apiBaseUrl', 'http://localhost:11434/v1')
    body = {
        'model': config.get('model', 'beam:cloud'),
        'messages': [{'role': 'user', 'content': prompt}],
        'temperature': config.get('temperature', 0.1),
        'max_tokens': config.get('max_tokens', 16000),
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
            if content.strip():
                return {'output': content}
            last_err = 'empty content from grader'
        except Exception as e:
            last_err = str(e)
        time.sleep(2 * (attempt + 1))
    return {'error': last_err or 'no content', 'output': ''}
