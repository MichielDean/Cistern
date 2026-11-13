"""Custom promptfoo grader provider for the cataractae eval suite.

Judge model, configured per-config in YAML (defaultTest.options.provider.config.model).
Defaults to glm-5.3-flash:cloud — selected by A/B (2026-11-13): best alignment with
the current rubric statements. Stronger non-GLM judges (beam, minimax-m3.1-flash,
kimi-k3.1-pro) grade cleanly after rubric disambiguation; revisit the default after
the full-grader matrix confirms. Same retry/empty-content handling as glm_flash.py.
"""
import json
import time
import urllib.request


def call_api(prompt, options, context):
    config = options.get('config', {}) or {}
    api_base = config.get('apiBaseUrl', 'http://localhost:11434/v1')
    body = {
        'model': config.get('model', 'glm-5.3-flash:cloud'),
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
