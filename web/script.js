document.addEventListener('DOMContentLoaded', () => {
    const downloadBtn = document.getElementById('downloadBtn');
    const urlInput = document.getElementById('urlInput');
    const statusDiv = document.getElementById('status');
    const resultArea = document.getElementById('resultArea');

    downloadBtn.addEventListener('click', async () => {
        const url = urlInput.value.trim();
        const format = document.querySelector('input[name="format"]:checked').value;

        if (!url) {
            showStatus('Please paste a valid URL', 'error');
            return;
        }

        // Reset UI
        setLoading(true);
        resultArea.style.display = 'none';

        try {
            const response = await fetch('/api/v1/download', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ url, format }),
            });

            const data = await response.json();

            if (!response.ok) {
                throw new Error(data.error || 'Download failed');
            }

            showStatus('Download Complete!', 'success');

            // Show result
            // The API returns "downloads/Title.mp4". Mapping /downloads -> ./downloads.
            const downloadUrl = '/' + data.file;

            const filePathDiv = resultArea.querySelector('.file-path');
            filePathDiv.innerHTML = `
                <a href="${downloadUrl}" download class="btn-download" style="display: block; text-decoration: none; text-align: center; margin-top: 10px; background: var(--success-color); color: #000;">
                    Click to Save File
                </a>
            `;
            resultArea.style.display = 'block';

        } catch (error) {
            showStatus(error.message, 'error');
        } finally {
            setLoading(false);
        }
    });

    function showStatus(msg, type) {
        statusDiv.textContent = msg;
        statusDiv.className = 'status-message visible ' + type;
    }

    function setLoading(isLoading) {
        if (isLoading) {
            downloadBtn.disabled = true;
            downloadBtn.innerHTML = '<span class="loader"></span> Processing...';
            statusDiv.className = 'status-message'; // hide
        } else {
            downloadBtn.disabled = false;
            downloadBtn.innerHTML = '<span>Download Now</span>';
        }
    }
});
