"""Custom promptfoo grader provider: model selectable via GLM_EVAL_GRADER env.

Defaults to beam:cloud — selected by judge A/B (2026-11-13): zero rubric
flakes across 18 graded assertions vs 1/18 for glm-5.3-flash and glm-5.3,
and a distinct model family from the glm subject (no self-grading bias).
Same retry/empty-content handling as glm_flash.py.
"""
import json
import os
import time
import urllib.request


def call_api(prompt, options, context):
    config = options.get('config', {}) or {}
    api_base = config.get('apiBaseUrl', 'http://localhost:11434/v1')
    body = {
        'model': os.environ.get('GLM_EVAL_GRADER', 'beam:cloud'),
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
