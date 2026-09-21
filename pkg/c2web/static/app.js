document.addEventListener('DOMContentLoaded', () => {
    const btnEval = document.getElementById('btn-eval');
    const codeEditor = document.getElementById('code-editor');
    const evalOutput = document.getElementById('eval-output');
    const startupBadge = document.getElementById('startup-badge');
    const execBadge = document.getElementById('exec-badge');
    const presets = document.querySelectorAll('.btn-preset');

    presets.forEach(btn => {
        btn.addEventListener('click', () => {
            codeEditor.value = btn.getAttribute('data-code');
        });
    });

    if (btnEval) {
        btnEval.addEventListener('click', async () => {
            btnEval.disabled = true;
            btnEval.textContent = 'Spawning Isolate & Executing...';
            execBadge.textContent = 'Exec: ...';

            const code = codeEditor.value;

            try {
                const res = await fetch('/api/v1/eval', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ code })
                });

                if (!res.ok) {
                    throw new Error(`HTTP ${res.status}`);
                }

                const data = await res.json();
                startupBadge.textContent = `Startup: ${data.startup_us.toFixed(1)} µs`;
                execBadge.textContent = `Exec: ${data.elapsed_us.toFixed(1)} µs`;

                if (data.success) {
                    evalOutput.style.color = '#10b981';
                    evalOutput.textContent = `// Isolate Execution Successful\n// Return Value:\n${data.result}`;
                } else {
                    evalOutput.style.color = '#ef4444';
                    evalOutput.textContent = `// Execution Error:\n${data.error}`;
                }
            } catch (err) {
                evalOutput.style.color = '#ef4444';
                evalOutput.textContent = `// Request Failed: ${err.message}`;
            } finally {
                btnEval.disabled = false;
                btnEval.textContent = 'Execute in New Isolate';
            }
        });
    }
});
