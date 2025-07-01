// Fetch all recordings from the server
async function fetchRecordings() {
    console.log("Fetching recordings list from server...");
    try {
        // Add cache-busting query parameter to prevent caching
        const timestamp = new Date().getTime();
        let response = await fetch(`/list-recordings?_=${timestamp}`);
        
        if (!response.ok) {
            console.error(`Failed to fetch recordings: ${response.status} ${response.statusText}`);
            throw new Error(`Failed to fetch recordings: ${response.status}`);
        }
        
        let recordings = await response.json();
        console.log(`Received ${recordings.length} recordings from server`);
        
        // Update the UI with the fetched recordings
        updateRecordingsList(recordings);
    } catch (error) {
        console.error("Error fetching recordings:", error);
        let listElement = document.getElementById("recordingsList");
        listElement.innerHTML = `<div class="empty-state">Could not load recordings. Please refresh the page.</div>`;
    }
}

// Update the recordings list in the UI
function updateRecordingsList(recordings) {
    let listElement = document.getElementById("recordingsList");
    
    if (!recordings || recordings.length === 0) {
        listElement.innerHTML = `<div class="empty-state">No recordings found. Start recording to create one!</div>`;
        return;
    }
    
    listElement.innerHTML = ""; // Clear previous list
    // No need to sort here, backend is already sorting by modification time (newest first)
    
    recordings.forEach(file => {
        let filename = file.split("/").pop();
        const date = formatRecordingDate(filename);
        
        let itemDiv = document.createElement("div");
        itemDiv.className = "record-item";
        
        // Create header for file info and main actions
        let itemHeader = document.createElement("div");
        itemHeader.className = "record-item-header";
        
        let fileInfo = document.createElement("a");
        fileInfo.href = "/" + file;
        fileInfo.download = filename;
        fileInfo.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="record-icon" viewBox="0 0 16 16">
                <path d="M6 13c0 1.105-1.12 2-2.5 2S1 14.105 1 13c0-1.104 1.12-2 2.5-2s2.5.896 2.5 2zm9-2c0 1.105-1.12 2-2.5 2s-2.5-.895-2.5-2 1.12-2 2.5-2 2.5.895 2.5 2z"/>
                <path fill-rule="evenodd" d="M14 11V2h1v9h-1zM6 3v10H5V3h1z"/>
                <path d="M5 2.905a1 1 0 0 1 .9-.995l8-.8a1 1 0 0 1 1.1.995V3L5 4V2.905z"/>
            </svg>
            <span>${date}</span>
        `;
        
        // Add audio player
        let audioPlayer = document.createElement("div");
        audioPlayer.className = "audio-player";
        audioPlayer.innerHTML = `
            <audio controls src="/${file}"></audio>
        `;
        
        // Create actions container
        let actions = document.createElement("div");
        actions.className = "actions";
        
        // Transcribe button
        let transcribeBtn = document.createElement("button");
        transcribeBtn.className = "btn-outline";
        transcribeBtn.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" fill="currentColor" viewBox="0 0 16 16">
                <path d="M2.114 8.063V7.9c1.005-.102 1.497-.615 1.497-1.6V4.503c0-1.094.39-1.538 1.354-1.538h.273V2h-.376C3.25 2 2.49 2.759 2.49 4.352v1.524c0 1.094-.376 1.456-1.49 1.456v1.299c1.114 0 1.49.362 1.49 1.456v1.524c0 1.593.759 2.352 2.372 2.352h.376v-.964h-.273c-.964 0-1.354-.444-1.354-1.538V9.663c0-.984-.492-1.497-1.497-1.6ZM13.886 7.9v.163c-1.005.103-1.497.616-1.497 1.6v1.798c0 1.094-.39 1.538-1.354 1.538h-.273v.964h.376c1.613 0 2.372-.759 2.372-2.352v-1.524c0-1.094.376-1.456 1.49-1.456V7.332c-1.114 0-1.49-.362-1.49-1.456V4.352C13.51 2.759 12.75 2 11.138 2h-.376v.964h.273c.964 0 1.354.444 1.354 1.538V6.3c0 .984.492 1.497 1.497 1.6Z"/>
            </svg>
            Transcribe
        `;
        transcribeBtn.onclick = (e) => {
            e.preventDefault();
            transcribeFile(filename, transcribeBtn);
        };
        
        // Prompt button
        let promptBtn = document.createElement("button");
        promptBtn.className = "btn-outline";
        promptBtn.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" fill="currentColor" viewBox="0 0 16 16">
                <path d="M15 2a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H1a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1h14zm-1 1H2v9h12V3z"/>
                <path d="M3 4.5a.5.5 0 0 1 1 0v7a.5.5 0 0 1-1 0v-7zm2 0a.5.5 0 0 1 1 0v7a.5.5 0 0 1-1 0v-7zm2 0a.5.5 0 0 1 1 0v7a.5.5 0 0 1-1 0v-7zm2 0a.5.5 0 0 1 .5-.5h1a.5.5 0 0 1 .5.5v7a.5.5 0 0 1-.5.5h-1a.5.5 0 0 1-.5-.5v-7z"/>
            </svg>
            Prompt
        `;
        promptBtn.onclick = (e) => {
            e.preventDefault();
            openPromptModal(filename);
        };
        
        // Rename button
        let renameBtn = document.createElement("button");
        renameBtn.className = "btn-outline";
        renameBtn.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" fill="currentColor" viewBox="0 0 16 16">
                <path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/>
            </svg>
            Rename
        `;
        renameBtn.onclick = (e) => {
            e.preventDefault();
            openRenameModal(filename);
        };
        
        // Delete button
        let deleteBtn = document.createElement("button");
        deleteBtn.className = "btn-danger";
        deleteBtn.innerHTML = `
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" fill="currentColor" viewBox="0 0 16 16">
                <path d="M5.5 5.5A.5.5 0 0 1 6 6v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5Zm2.5 0a.5.5 0 0 1 .5.5v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5Zm3 .5a.5.5 0 0 0-1 0v6a.5.5 0 0 0 1 0V6Z"/>
                <path d="M14.5 3a1 1 0 0 1-1 1H13v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V4h-.5a1 1 0 0 1-1-1V2a1 1 0 0 1 1-1H6a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1h3.5a1 1 0 0 1 1 1v1ZM4.118 4 4 4.059V13a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1V4.059L11.882 4H4.118ZM2.5 3h11V2h-11v1Z"/>
            </svg>
            Delete
        `;
        deleteBtn.onclick = (e) => {
            e.preventDefault();
            openDeleteModal(filename);
        };
        
        // Add all buttons to actions
        actions.appendChild(transcribeBtn);
        actions.appendChild(promptBtn);
        actions.appendChild(renameBtn);
        actions.appendChild(deleteBtn);
        
        // Build the layout
        itemHeader.appendChild(fileInfo);
        itemDiv.appendChild(itemHeader);
        itemDiv.appendChild(audioPlayer);
        itemDiv.appendChild(actions);
        listElement.appendChild(itemDiv);
        
        // Check if a transcript exists
        checkTranscript(filename, itemDiv);
    });
}

// Check if a transcript exists for this recording
function checkTranscript(filename, itemDiv) {
    const mdFilename = filename.replace(/\.[^/.]+$/, ".md");
    fetch(`/transcript?filename=${encodeURIComponent(filename)}`)
        .then(response => {
            if (response.ok) {
                return response.text();
            }
            return null;
        })
        .then(data => {
            if (data && data !== `Hello ${filename}`) {
                // Store the transcript content but don't show it inline anymore
                // Save a data attribute on the button instead
                const transcribeBtn = itemDiv.querySelector('.btn-outline');
                if (transcribeBtn) {
                    transcribeBtn.dataset.transcript = data;
                    // Modify the button to indicate a transcript is available, but keep it clickable
                    transcribeBtn.innerHTML = `
                        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" fill="currentColor" viewBox="0 0 16 16">
                            <path d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zm-3.97-3.03a.75.75 0 0 0-1.08.022L7.477 9.417 5.384 7.323a.75.75 0 0 0-1.06 1.06L6.97 11.03a.75.75 0 0 0 1.079-.02l3.992-4.99a.75.75 0 0 0-.01-1.05z"/>
                        </svg>
                        View Transcript
                    `;
                    
                    // Update the onclick handler to open the modal with the stored transcript
                    transcribeBtn.onclick = (e) => {
                        e.preventDefault();
                        openContentModal(`Transcript: ${filename}`, data);
                    };
                }
            }
        })
        .catch(err => {
            console.error("Error checking transcript:", err);
        });
}

function transcribeFile(filename, button) {
    button.disabled = true;
    button.innerHTML = `
        <span>Transcribing</span>
        <span class="loader"></span>
    `;
    
    fetch(`/transcript?filename=${encodeURIComponent(filename)}`)
        .then(async response => {
            // Store the response for error handling
            const responseText = await response.text();
            
            if (!response.ok) {
                // Get the detailed error message from the response
                let errorMessage = "Transcription failed";
                
                if (responseText && responseText.length > 0) {
                    // This is likely the error message from the server
                    errorMessage = responseText;
                } else if (response.status === 404) {
                    errorMessage = "Audio file not found. It may have been deleted.";
                } else if (response.status === 400) {
                    errorMessage = "Invalid audio file. The format may not be supported.";
                } else if (response.status === 500) {
                    errorMessage = "Server error during transcription. Please try again later.";
                }
                
                throw new Error(errorMessage);
            }
            
            return responseText;
        })
        .then(data => {
            // Reset the button state
            button.innerHTML = `
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" fill="currentColor" viewBox="0 0 16 16">
                    <path d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zm-3.97-3.03a.75.75 0 0 0-1.08.022L7.477 9.417 5.384 7.323a.75.75 0 0 0-1.06 1.06L6.97 11.03a.75.75 0 0 0 1.079-.02l3.992-4.99a.75.75 0 0 0-.01-1.05z"/>
                </svg>
                View Transcript
            `;
            button.disabled = false;
            
            // Store the transcript on the button for easy access
            button.dataset.transcript = data;
            
            // Update onclick to view the saved transcript
            button.onclick = (e) => {
                e.preventDefault();
                openContentModal(`Transcript: ${filename}`, data);
            };
            
            // Open content modal with transcript
            openContentModal(`Transcript: ${filename}`, data);
        })
        .catch(error => {
            console.error("Transcription error:", error);
            
            // Reset the button
            button.innerHTML = `
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" fill="currentColor" viewBox="0 0 16 16">
                    <path d="M2.114 8.063V7.9c1.005-.102 1.497-.615 1.497-1.6V4.503c0-1.094.39-1.538 1.354-1.538h.273V2h-.376C3.25 2 2.49 2.759 2.49 4.352v1.524c0 1.094-.376 1.456-1.49 1.456v1.299c1.114 0 1.49.362 1.49 1.456v1.524c0 1.593.759 2.352 2.372 2.352h.376v-.964h-.273c-.964 0-1.354-.444-1.354-1.538V9.663c0-.984-.492-1.497-1.497-1.6ZM13.886 7.9v.163c-1.005.103-1.497.616-1.497 1.6v1.798c0 1.094-.39 1.538-1.354 1.538h-.273v.964h.376c1.613 0 2.372-.759 2.372-2.352v-1.524c0-1.094.376-1.456 1.49-1.456V7.332c-1.114 0-1.49-.362-1.49-1.456V4.352C13.51 2.759 12.75 2 11.138 2h-.376v.964h.273c.964 0 1.354.444 1.354 1.538V6.3c0 .984.492 1.497 1.497 1.6Z"/>
                </svg>
                Transcribe
            `;
            button.disabled = false;
            
            // Show the error in a modal instead of an alert
            let errorMessage = error.message || "Failed to transcribe recording. Please try again.";
            
            // Create a formatted error message for the modal
            const formattedError = `
## Transcription Error

${errorMessage}

### Troubleshooting

- Check that the audio file contains clear speech
- System audio recordings work best with the WebM format
- Verify your Google Cloud configuration
- Make sure your GCP project has the necessary APIs enabled
            `;
            
            openContentModal(`Error Transcribing: ${filename}`, formattedError);
        });
}