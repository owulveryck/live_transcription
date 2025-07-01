// Variables for prompt, rename, and delete handling
let currentPromptFilename = '';
let currentRenameFilename = '';
let currentDeleteFilename = '';

// Open the prompt modal
function openPromptModal(filename) {
    currentPromptFilename = filename;
    document.getElementById('promptText').value = '';
    document.getElementById('promptModal').classList.add('active');
}

// Close the prompt modal
function closePromptModal() {
    document.getElementById('promptModal').classList.remove('active');
}

// Open the rename modal
function openRenameModal(filename) {
    currentRenameFilename = filename;
    
    // Extract filename without extension and populate the input field
    const nameWithoutExt = filename.replace(/\.[^/.]+$/, "");
    document.getElementById('newFilename').value = nameWithoutExt;
    document.getElementById('renameModal').classList.add('active');
    document.getElementById('newFilename').focus();
}

// Close the rename modal
function closeRenameModal() {
    document.getElementById('renameModal').classList.remove('active');
}

// Submit the rename request
async function submitRename() {
    const newName = document.getElementById('newFilename').value.trim();
    
    if (!newName) {
        alert('Please enter a new name');
        return;
    }
    
    const submitBtn = document.getElementById('submitRenameBtn');
    const originalText = submitBtn.innerHTML;
    
    // Show loading state
    submitBtn.disabled = true;
    submitBtn.innerHTML = `
        <span>Renaming</span>
        <span class="loader"></span>
    `;
    
    try {
        const response = await fetch('/rename-recording', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                oldFilename: currentRenameFilename,
                newFilename: newName
            })
        });
        
        if (!response.ok) {
            throw new Error('Failed to rename recording');
        }
        
        // Close the rename modal
        closeRenameModal();
        
        // Refresh the recordings list
        fetchRecordings();
    } catch (error) {
        console.error('Error renaming recording:', error);
        alert('Failed to rename recording. Please try again.');
    } finally {
        // Reset button state
        submitBtn.disabled = false;
        submitBtn.innerHTML = originalText;
    }
}

// Open the content display modal
function openContentModal(title, content, isMarkdown = true) {
    document.getElementById('contentModalTitle').textContent = title;
    const markdownContainer = document.getElementById('markdown-container');
    
    if (isMarkdown) {
        // Simple markdown parsing for basic formatting
        let formattedContent = content
            .replace(/^# (.*$)/gm, '<h1>$1</h1>')
            .replace(/^## (.*$)/gm, '<h2>$1</h2>')
            .replace(/^### (.*$)/gm, '<h3>$1</h3>')
            .replace(/^#### (.*$)/gm, '<h4>$1</h4>')
            .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
            .replace(/\*(.*?)\*/g, '<em>$1</em>')
            .replace(/`(.*?)`/g, '<code>$1</code>')
            .replace(/^\> (.*$)/gm, '<blockquote>$1</blockquote>')
            .replace(/^- (.*$)/gm, '<ul><li>$1</li></ul>')
            .replace(/<\/ul><ul>/g, '');
        
        markdownContainer.innerHTML = formattedContent;
    } else {
        markdownContainer.textContent = content;
    }
    
    document.getElementById('contentModal').classList.add('active');
}

// Close the content display modal
function closeContentModal() {
    document.getElementById('contentModal').classList.remove('active');
}

// Copy content from the markdown container to clipboard
function copyMarkdownContent() {
    const markdownContent = document.getElementById('markdown-container').textContent;
    navigator.clipboard.writeText(markdownContent)
        .then(() => {
            const copyBtn = document.getElementById('copyContentBtn');
            const originalText = copyBtn.innerHTML;
            
            copyBtn.innerHTML = `
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16">
                    <path d="M13.854 3.646a.5.5 0 0 1 0 .708l-7 7a.5.5 0 0 1-.708 0l-3.5-3.5a.5.5 0 1 1 .708-.708L6.5 10.293l6.646-6.647a.5.5 0 0 1 .708 0z"/>
                </svg>
                Copied!
            `;
            
            setTimeout(() => {
                copyBtn.innerHTML = originalText;
            }, 2000);
        })
        .catch(err => {
            console.error('Failed to copy: ', err);
            alert('Failed to copy to clipboard');
        });
}

// Open the delete confirmation modal
function openDeleteModal(filename) {
    currentDeleteFilename = filename;
    document.getElementById('deleteFileName').textContent = filename;
    document.getElementById('deleteModal').classList.add('active');
}

// Close the delete confirmation modal
function closeDeleteModal() {
    document.getElementById('deleteModal').classList.remove('active');
}

// Submit the delete request
async function confirmDelete() {
    if (!currentDeleteFilename) {
        alert('No file selected for deletion');
        return;
    }
    
    const confirmBtn = document.getElementById('confirmDeleteBtn');
    const originalText = confirmBtn.innerHTML;
    
    // Show loading state
    confirmBtn.disabled = true;
    confirmBtn.innerHTML = `
        <span>Deleting</span>
        <span class="loader"></span>
    `;
    
    try {
        const response = await fetch('/delete-recording', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                filename: currentDeleteFilename
            })
        });
        
        if (!response.ok) {
            throw new Error('Failed to delete recording');
        }
        
        // Close the delete modal
        closeDeleteModal();
        
        // Refresh the recordings list
        fetchRecordings();
    } catch (error) {
        console.error('Error deleting recording:', error);
        alert('Failed to delete recording. Please try again.');
    } finally {
        // Reset button state
        confirmBtn.disabled = false;
        confirmBtn.innerHTML = originalText;
    }
}

// Submit the prompt to the server
async function submitPrompt() {
    const promptText = document.getElementById('promptText').value.trim();
    
    if (!promptText) {
        alert('Please enter a prompt');
        return;
    }
    
    const submitBtn = document.getElementById('submitPromptBtn');
    const originalText = submitBtn.innerHTML;
    
    // Show loading state
    submitBtn.disabled = true;
    submitBtn.innerHTML = `
        <span>Submitting</span>
        <span class="loader"></span>
    `;
    
    try {
        const response = await fetch('/prompt', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                filename: currentPromptFilename,
                prompt: promptText
            })
        });
        
        if (!response.ok) {
            throw new Error('Failed to submit prompt');
        }
        
        const data = await response.text();
        
        // Close the prompt modal
        closePromptModal();
        
        // Open the content modal with the result
        openContentModal(`Prompt Result: ${currentPromptFilename}`, data);
    } catch (error) {
        console.error('Error submitting prompt:', error);
        alert('Failed to submit prompt. Please try again.');
    } finally {
        // Reset button state
        submitBtn.disabled = false;
        submitBtn.innerHTML = originalText;
    }
}