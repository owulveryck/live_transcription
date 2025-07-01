// Initialize visualizer
function initVisualizer() {
    const visualizerBars = document.getElementById('visualizerBars');
    visualizerBars.innerHTML = '';
    
    // Create 30 bars for the visualizer
    for (let i = 0; i < 30; i++) {
        const bar = document.createElement('div');
        bar.className = 'visualizer-bar';
        visualizerBars.appendChild(bar);
    }
}

// Update the recording time counter
function updateRecordingTime() {
    const elapsed = Math.floor((Date.now() - recordingStartTime) / 1000);
    const minutes = Math.floor(elapsed / 60).toString().padStart(2, '0');
    const seconds = (elapsed % 60).toString().padStart(2, '0');
    document.getElementById('recordingTime').textContent = `${minutes}:${seconds}`;
}

// Update visualizer with current audio data
function updateVisualizer() {
    if (!analyser) return;
    
    const bufferLength = analyser.frequencyBinCount;
    const dataArray = new Uint8Array(bufferLength);
    analyser.getByteFrequencyData(dataArray);
    
    const bars = document.querySelectorAll('.visualizer-bar');
    const step = Math.floor(bufferLength / bars.length);
    
    for (let i = 0; i < bars.length; i++) {
        const value = dataArray[i * step];
        const height = Math.max(5, value / 255 * 100);
        bars[i].style.height = `${height}%`;
    }
}

// Populate the audio input devices dropdown
async function populateAudioDevices() {
    const audioSelect = document.getElementById('audioSource');
    
    try {
        // First, get permission to access media devices
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
        stream.getTracks().forEach(track => track.stop()); // Release the stream immediately
        
        // Now enumerate devices
        const devices = await navigator.mediaDevices.enumerateDevices();
        
        // Clear previous options
        audioSelect.innerHTML = '';
        
        // Add a default option
        const defaultOption = document.createElement('option');
        defaultOption.value = 'default';
        defaultOption.text = 'Default Microphone';
        defaultOption.selected = true;
        audioSelect.appendChild(defaultOption);
        
        // Only show the audio source selector when microphone is selected
        updateAudioSourceVisibility();
        
        // Add options for audio input devices
        const audioInputDevices = devices.filter(device => device.kind === 'audioinput');
        audioInputDevices.forEach(device => {
            const option = document.createElement('option');
            option.value = device.deviceId;
            option.text = device.label || `Microphone ${audioSelect.length + 1}`;
            audioSelect.appendChild(option);
        });
        
        console.log(`Found ${audioInputDevices.length} audio input devices`);
    } catch (err) {
        console.error('Error enumerating audio devices:', err);
        
        // Add a fallback option
        const fallbackOption = document.createElement('option');
        fallbackOption.value = 'default';
        fallbackOption.text = 'Default Microphone';
        fallbackOption.selected = true;
        audioSelect.appendChild(fallbackOption);
    }
}

async function startRecording() {
    document.getElementById("startBtn").disabled = true;
    document.getElementById("stopBtn").disabled = false;
    document.getElementById("status").classList.remove("hidden");
    document.getElementById("audioVisualizer").classList.remove("hidden");
    
    // Get the optional recording name
    const recordingName = document.getElementById("recordingName").value.trim();
    
    // Initialize recording time counter
    recordingStartTime = Date.now();
    recordingTimer = setInterval(updateRecordingTime, 1000);
    
    // Initialize visualizer
    initVisualizer();
    
    try {
        // Get the selected recording mode
        const recordingMode = document.querySelector('input[name="recordingMode"]:checked').value;
        
        // Show an informational message when system audio is selected
        if (recordingMode === 'system' || recordingMode === 'both') {
            console.info("User selected system audio recording mode");
            const systemAudioSupported = isSystemAudioSupported();
            if (!systemAudioSupported) {
                document.getElementById('status').innerHTML = `
                    <span class="recording-dot"></span>
                    <span>System audio not supported in this browser. Please try Chrome or Edge.</span>
                    <div id="recordingTime" class="recording-time">00:00</div>
                `;
            }
        }
        
        // Collect audio streams based on selected mode
        if (recordingMode === 'microphone' || recordingMode === 'both') {
            // Get the selected audio device for microphone
            const audioSelect = document.getElementById('audioSource');
            const micConstraints = { 
                audio: {
                    deviceId: audioSelect.value !== 'default' ? { exact: audioSelect.value } : undefined,
                    echoCancellation: true,
                    noiseSuppression: true,
                    autoGainControl: true
                }
            };
            
            console.log('Using audio device:', audioSelect.options[audioSelect.selectedIndex].text);
            micStream = await navigator.mediaDevices.getUserMedia(micConstraints);
        }
        
        // Handle system audio if needed
        if (recordingMode === 'system' || recordingMode === 'both') {
            // Check browser support for system audio capture
            if (!isSystemAudioSupported()) {
                if (recordingMode === 'system') {
                    throw new Error("Your browser doesn't support system audio capture. Try using Chrome or Edge browser.");
                } else {
                    alert("System audio capture is not supported in your browser. Recording microphone only.");
                }
            } else {
                // Request system audio (this will prompt the user to share their screen/audio)
                try {
                    // Basic displayMedia request that works more consistently
                    systemStream = await navigator.mediaDevices.getDisplayMedia({ 
                        video: true,  // Need video: true for many browsers (we'll disable it later)
                        audio: true
                    });
                    
                    // Check if user actually shared audio
                    const hasAudio = systemStream.getAudioTracks().length > 0;
                    if (!hasAudio) {
                        alert("No system audio was shared. Make sure to select 'Share audio' when prompted.");
                        if (recordingMode === 'system') {
                            throw new Error("No system audio was shared");
                        }
                    } else {
                        console.log('System audio capture enabled');
                        console.log('System audio tracks:', systemStream.getAudioTracks().length);
                        
                        // Turn off video tracks as we only need audio
                        systemStream.getVideoTracks().forEach(track => {
                            track.enabled = false;
                        });
                    }
                } catch (err) {
                    console.error("Error getting system audio:", err);
                    if (recordingMode === 'system') {
                        // If only system audio was requested and it failed, we need to stop
                        if (err.name === 'NotAllowedError') {
                            throw new Error("Permission denied. Please allow screen sharing with audio.");
                        } else if (err.name === 'NotSupportedError') {
                            throw new Error("System audio capture is not supported in your browser configuration.");
                        } else {
                            throw err;
                        }
                    } else {
                        // For 'both' mode, we can continue with just the mic
                        alert("Could not capture system audio. Recording microphone only.");
                    }
                }
            }
        }
        
        // Combine streams if needed
        let finalStream;
        if (recordingMode === 'both' && micStream && systemStream && systemStream.getAudioTracks().length > 0) {
            try {
                console.log("Mixing microphone and system audio streams");
                
                // Log info about source streams before mixing
                console.log("Source stream info:");
                console.log("- Microphone stream tracks:", micStream.getTracks().map(t => `${t.kind}:${t.label}`));
                console.log("- System stream tracks:", systemStream.getTracks().map(t => `${t.kind}:${t.label}`));
                
                // Create audio context for proper mixing
                const mixingContext = new AudioContext();
                
                // Create media stream destination for mixed output
                const dest = mixingContext.createMediaStreamDestination();
                
                // Connect microphone to the destination
                if (micStream && micStream.getAudioTracks().length > 0) {
                    const micSource = mixingContext.createMediaStreamSource(micStream);
                    micSource.connect(dest);
                    console.log("Connected microphone to audio mixer");
                }
                
                // Connect system audio to the destination
                if (systemStream && systemStream.getAudioTracks().length > 0) {
                    const sysSource = mixingContext.createMediaStreamSource(systemStream);
                    sysSource.connect(dest);
                    console.log("Connected system audio to audio mixer");
                }
                
                // Use the destination stream which contains the mixed audio
                finalStream = dest.stream;
                console.log("Created mixed audio stream with tracks:", finalStream.getAudioTracks().length);
                
                // Log detailed info about the mixed track
                finalStream.getAudioTracks().forEach((track, i) => {
                    console.log(`Mixed stream track ${i}: ${track.kind}, Label: ${track.label}, Enabled: ${track.enabled}`);
                });
                
            } catch (error) {
                console.error("Error mixing audio streams:", error);
                // Fallback to just microphone if mixing fails
                alert("Error mixing audio streams. Falling back to microphone only.");
                finalStream = micStream;
            }
        } else if (recordingMode === 'system' && systemStream) {
            finalStream = systemStream;
        } else {
            finalStream = micStream;
        }
        
        // Set up analyser node for visualizer
        audioContext = new AudioContext({ sampleRate: 44100 });
        let source = audioContext.createMediaStreamSource(finalStream);
        analyser = audioContext.createAnalyser();
        analyser.fftSize = 256;
        source.connect(analyser);
        
        // Start visualizer updates with lower refresh rate (less CPU intensive)
        visualizerTimer = setInterval(updateVisualizer, 100);
        
        // Use WAV format if possible for maximum compatibility especially with system audio
        let options = {};
        let useBlobWorkaround = false;
        
        // For system audio, we'll use Opus which is optimized for voice and has good compatibility
        if (recordingMode === 'system' || recordingMode === 'both') {
            const systemAudioFormats = [
                'audio/webm;codecs=opus',     // WebM with Opus codec (best for voice)
                'audio/ogg;codecs=opus',      // Ogg with Opus codec (alternative container)
                'audio/opus',                 // Direct Opus format
                'audio/webm',                 // WebM with default codec
                ''                            // Empty string = browser's default format
            ];
            
            for (const mimeType of systemAudioFormats) {
                if (!mimeType || MediaRecorder.isTypeSupported(mimeType)) {
                    options.mimeType = mimeType;
                    console.log(`Selected system audio format: ${mimeType || "browser default"}`);
                    
                    // Debug info - log all supported formats
                    if (mimeType === 'audio/webm;codecs=vorbis') {
                        console.log("🎉 Using Vorbis codec in WebM container");
                    } else {
                        console.warn("⚠️ Not using Vorbis. Testing for Vorbis support:");
                        console.log("- audio/webm;codecs=vorbis supported:", MediaRecorder.isTypeSupported("audio/webm;codecs=vorbis"));
                        console.log("- audio/ogg;codecs=vorbis supported:", MediaRecorder.isTypeSupported("audio/ogg;codecs=vorbis"));
                        console.log("- audio/webm;codecs=opus supported:", MediaRecorder.isTypeSupported("audio/webm;codecs=opus"));
                    }
                    
                    break;
                }
            }
        } else {
            // For microphone, use Opus which is optimized for voice
            const mimeTypes = [
                'audio/webm;codecs=opus',     // WebM with Opus codec (best for voice)
                'audio/ogg;codecs=opus',      // Ogg with Opus codec
                'audio/opus',                 // Direct Opus format
                'audio/webm',                 // WebM with default codec
                'audio/ogg',                  // Ogg with default codec
                'audio/wav',                  // Uncompressed WAV (high quality but large)
                'audio/webm;codecs=vorbis',   // WebM with Vorbis codec
                'audio/ogg;codecs=vorbis',    // Ogg with Vorbis codec 
                'audio/mp3',                  // MP3 format
                'audio/mpeg',                 // MP3 format (alternative MIME type)
                ''                            // Empty string = browser's default format
            ];
            
            for (const mimeType of mimeTypes) {
                if (!mimeType || MediaRecorder.isTypeSupported(mimeType)) {
                    options.mimeType = mimeType;
                    break;
                }
            }
        }
        
        console.log("Using audio format:", options.mimeType || "browser default");
        
        // Ensure we have a valid stream with audio tracks
        if (!finalStream || finalStream.getAudioTracks().length === 0) {
            throw new Error("No audio tracks available in the stream");
        }
        
        console.log("Audio tracks:", finalStream.getAudioTracks().length);
        
        try {
            console.log("Creating MediaRecorder with stream tracks:", finalStream.getTracks().map(t => t.kind).join(', '));
            
            // Special handling for system audio recording
            if (recordingMode === 'system' && systemStream) {
                // Use just the system audio track for system-only mode
                const audioTracks = [];
                
                if (systemStream && systemStream.getAudioTracks().length > 0) {
                    audioTracks.push(systemStream.getAudioTracks()[0]);
                    console.log("Added system audio track to audioOnlyStream");
                }
                
                if (audioTracks.length > 0) {
                    // Create a clean stream with just the system audio track
                    const systemOnlyStream = new MediaStream(audioTracks);
                    console.log(`Created system-only stream with ${systemOnlyStream.getAudioTracks().length} audio tracks`);
                    finalStream = systemOnlyStream;
                }
            } else if (recordingMode === 'both' && finalStream && finalStream.getAudioTracks().length >= 2) {
                // For 'both' mode, we already have a combined stream from earlier code
                console.log(`Using combined stream with ${finalStream.getAudioTracks().length} audio tracks`);
                finalStream.getAudioTracks().forEach((track, i) => {
                    console.log(`Combined track ${i}: ${track.kind}, Label: ${track.label}`);
                });
            }
            
            // Ensure we have audio tracks in our stream
            if (!finalStream || finalStream.getAudioTracks().length === 0) {
                throw new Error("No audio tracks available in the stream");
            }
            
            // Create the MediaRecorder with appropriate format
            try {
                // Try with Opus first (best for voice)
                if (MediaRecorder.isTypeSupported('audio/webm;codecs=opus')) {
                    mediaRecorder = new MediaRecorder(finalStream, {mimeType: 'audio/webm;codecs=opus'});
                    console.log("🎉 Successfully created MediaRecorder with WebM Opus format");
                } 
                else if (MediaRecorder.isTypeSupported('audio/ogg;codecs=opus')) {
                    mediaRecorder = new MediaRecorder(finalStream, {mimeType: 'audio/ogg;codecs=opus'});
                    console.log("🎉 Successfully created MediaRecorder with Ogg Opus format");
                }
                else if (MediaRecorder.isTypeSupported('audio/opus')) {
                    mediaRecorder = new MediaRecorder(finalStream, {mimeType: 'audio/opus'});
                    console.log("🎉 Successfully created MediaRecorder with Opus format");
                }
                else {
                    // Fall back to browser's default or specified options
                    mediaRecorder = new MediaRecorder(finalStream, options);
                    console.log("Created MediaRecorder with format:", options.mimeType || "browser default");
                }
                
                // Verify the MediaRecorder is using the correct stream
                console.log("MediaRecorder stream check:", {
                    recordingMode: recordingMode,
                    audioTracksInRecorder: mediaRecorder.stream.getAudioTracks().length,
                    expectedTracks: finalStream.getAudioTracks().length
                });
                
                // Log what format was actually selected by the browser
                console.log("MediaRecorder actual mime type:", mediaRecorder.mimeType);
                
            } catch (err) {
                console.error("Error creating MediaRecorder:", err);
                throw err;
            }
            
            console.log("MediaRecorder created successfully");
            
            // Set up data handler - make this more robust
            mediaRecorder.ondataavailable = (event) => {
                try {
                    if (event.data && event.data.size > 0 && socket && socket.readyState === WebSocket.OPEN) {
                        // Log information about the audio data
                        if (mediaRecorder.ondataavailable.called) {
                            // Just log the size for subsequent chunks to avoid flooding console
                            console.log(`Sent audio chunk: ${event.data.size} bytes`);
                        } else {
                            // Get the actual format from the first chunk
                            let actualFormat = event.data.type || mediaRecorder.mimeType || 'audio/webm;codecs=opus';
                            
                            // Normalize Opus formats for consistency
                            if (actualFormat.includes("opus")) {
                                if (actualFormat.includes("webm")) {
                                    actualFormat = "audio/webm;codecs=opus";
                                } else if (actualFormat.includes("ogg")) {
                                    actualFormat = "audio/ogg;codecs=opus";
                                } else {
                                    actualFormat = "audio/opus";
                                }
                            }
                            // Normalize MP3 formats (legacy support)
                            else if (actualFormat === "audio/mpeg" || actualFormat.includes("mp3")) {
                                actualFormat = "audio/mp3";
                            }
                            
                            // For the first chunk, log more detailed info
                            console.log(`First audio chunk data:`, {
                                size: event.data.size + " bytes",
                                type: event.data.type,
                                format: mediaRecorder.mimeType,
                                actualFormat: actualFormat,
                                recordingMode: recordingMode
                            });
                            
                            // Call our format update function with the actual format
                            if (typeof mediaRecorder.onFirstChunk === 'function') {
                                mediaRecorder.onFirstChunk(actualFormat);
                            }
                            
                            // Mark that we've called this function
                            mediaRecorder.ondataavailable.called = true;
                        }
                        
                        // Send the audio data
                        socket.send(event.data);
                    }
                } catch (err) {
                    console.error("Error sending audio data:", err);
                }
            };
            
            // Make sure the property exists
            mediaRecorder.ondataavailable.called = false;
            
            // Use a try-catch block specifically for start()
            try {
                // Start recording with smaller chunks
                mediaRecorder.start(250); // Use 250ms chunks for better stability
                console.log("MediaRecorder started successfully");
            } catch (startErr) {
                console.error("Error starting MediaRecorder:", startErr);
                throw startErr;
            }
        } catch (err) {
            console.error("MediaRecorder error:", err);
            
            // One last attempt with the simplest possible configuration
            if ((recordingMode === 'system' || recordingMode === 'both') && systemStream) {
                try {
                    console.log("Trying last-resort approach for system audio");
                    // Get just the first audio track
                    const audioTrack = systemStream.getAudioTracks()[0];
                    if (audioTrack) {
                        const singleTrackStream = new MediaStream([audioTrack]);
                        
                        // Create with explicit Opus options if supported
                        try {
                            if (MediaRecorder.isTypeSupported('audio/webm;codecs=opus')) {
                                mediaRecorder = new MediaRecorder(singleTrackStream, {mimeType: 'audio/webm;codecs=opus'});
                                console.log("🎉 Successfully created fallback MediaRecorder with WebM Opus format");
                            } 
                            else if (MediaRecorder.isTypeSupported('audio/ogg;codecs=opus')) {
                                mediaRecorder = new MediaRecorder(singleTrackStream, {mimeType: 'audio/ogg;codecs=opus'});
                                console.log("🎉 Successfully created fallback MediaRecorder with Ogg Opus format");
                            }
                            else if (MediaRecorder.isTypeSupported('audio/opus')) {
                                mediaRecorder = new MediaRecorder(singleTrackStream, {mimeType: 'audio/opus'});
                                console.log("🎉 Successfully created fallback MediaRecorder with Opus format");
                            }
                            else {
                                // No options = browser's default
                                mediaRecorder = new MediaRecorder(singleTrackStream);
                                console.log("⚠️ Created fallback MediaRecorder with browser's default format");
                            }
                        } catch (err) {
                            console.warn("Could not create MediaRecorder with preferred format, using browser defaults:", err);
                            mediaRecorder = new MediaRecorder(singleTrackStream);
                        }
                        
                        // Log what was actually selected
                        console.log("MediaRecorder created with mime type:", mediaRecorder.mimeType);
                        
                        finalStream = singleTrackStream;
                        
                        // Set data handler
                        mediaRecorder.ondataavailable = (event) => {
                            if (event.data.size > 0 && socket && socket.readyState === WebSocket.OPEN) {
                                console.log(`Sending ${event.data.size} bytes of data, type:`, event.data.type);
                                socket.send(event.data);
                            }
                        };
                        
                        // Start recording
                        mediaRecorder.start(100);
                        console.log("Successfully created MediaRecorder with fallback approach");
                        return; // Skip the error throw
                    }
                } catch (fallbackErr) {
                    console.error("Even the fallback approach failed:", fallbackErr);
                }
            }
            
            throw new Error("Failed to initialize MediaRecorder: " + err.message);
        }
        
        // Use current window location to determine WebSocket URL
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/stream`;
        
        // Add recording name as query parameter if provided
        const recordingName = document.getElementById("recordingName").value.trim();
        const wsUrlWithName = recordingName ? `${wsUrl}?name=${encodeURIComponent(recordingName)}` : wsUrl;
        
        socket = new WebSocket(wsUrlWithName);
        
        // Use arraybuffer for binary type - this is more reliable for audio data
        socket.binaryType = "arraybuffer";
        
        // Wait for socket to open before starting MediaRecorder
        socket.onopen = () => {
            console.log("WebSocket connection established");
            
            // We need to wait for the first audio chunk to know the actual format
            // So let's define a function to be called once we have the first chunk
            mediaRecorder.onFirstChunk = (actualFormat) => {
                console.log("📋 Actual format detected from first chunk:", actualFormat);
                
                // Normalize format names for consistency
                let normalizedFormat = actualFormat;
                
                // Normalize Opus formats (can be reported in different ways)
                if (actualFormat.includes("opus")) {
                    if (actualFormat.includes("webm")) {
                        normalizedFormat = "audio/webm;codecs=opus";
                    } else if (actualFormat.includes("ogg")) {
                        normalizedFormat = "audio/ogg;codecs=opus";
                    } else {
                        normalizedFormat = "audio/opus";
                    }
                }
                // MP3 formats (legacy support)
                else if (actualFormat === "audio/mpeg" || actualFormat.includes("mp3")) {
                    normalizedFormat = "audio/mp3";
                }
                
                // Send format information to the server with the ACTUAL format
                const formatInfo = {
                    format: normalizedFormat,
                    sampleRate: 44100,
                    channels: 1
                };
                
                console.log("Sending format info to server:", formatInfo);
                socket.send(JSON.stringify(formatInfo));
            };
            
            // Get initial format being used from the MediaRecorder instance
            // This might change when we get the first data chunk
            let initialFormat = mediaRecorder.mimeType || "audio/webm;codecs=opus";
            console.log("📋 MediaRecorder reports initial mime type:", initialFormat);
            
            // If no MIME type is reported, determine from browser
            if (!initialFormat || initialFormat === "") {
                // Use Opus as fallback format when possible
                if (MediaRecorder.isTypeSupported('audio/webm;codecs=opus')) {
                    initialFormat = "audio/webm;codecs=opus";
                } else if (MediaRecorder.isTypeSupported('audio/ogg;codecs=opus')) {
                    initialFormat = "audio/ogg;codecs=opus";
                } else if (MediaRecorder.isTypeSupported('audio/opus')) {
                    initialFormat = "audio/opus";
                } else {
                    // Last resort fallback
                    initialFormat = "audio/webm";
                }
                console.log("📋 No MIME type reported initially, using fallback:", initialFormat);
            }
            
            // Normalize the initial format for consistency
            if (initialFormat.includes("opus")) {
                if (initialFormat.includes("webm")) {
                    initialFormat = "audio/webm;codecs=opus";
                } else if (initialFormat.includes("ogg")) {
                    initialFormat = "audio/ogg;codecs=opus";
                } else {
                    initialFormat = "audio/opus";
                }
            }
            
            // Send initial format info (will be updated when we get the first chunk)
            const initialFormatInfo = {
                format: initialFormat,
                sampleRate: 44100,
                channels: 1
            };
            
            console.log("Sending initial format info to server:", initialFormatInfo);
            socket.send(JSON.stringify(initialFormatInfo));
        };
        
        socket.onmessage = (event) => {
            try {
                let data = JSON.parse(event.data);
                console.log("Received data from server:", data);
                
                // Check if the data is an array (recording list)
                if (Array.isArray(data)) {
                    console.log("Received recordings list. Updating UI...");
                    updateRecordingsList(data);
                } else {
                    console.log("Received non-array data:", data);
                }
            } catch (error) {
                console.error("Error parsing server message:", error, event.data);
            }
        };
        
        socket.onerror = (error) => {
            console.error("WebSocket error:", error);
            stopRecording();
            alert("Connection error. Please try again.");
        };
        
        // Add an explicit onclose handler to fetch recordings when connection closes
        socket.onclose = (event) => {
            console.log("WebSocket connection closed", event);
            // Make sure we update the UI with the latest recordings
            setTimeout(fetchRecordings, 800);
        };
    } catch (error) {
        console.error("Error starting recording:", error);
        document.getElementById("startBtn").disabled = false;
        document.getElementById("stopBtn").disabled = true;
        document.getElementById("status").classList.add("hidden");
        document.getElementById("audioVisualizer").classList.add("hidden");
        
        // Provide a more specific error message based on the error type
        let errorMessage = "Recording failed: ";
        
        if (error.name === "NotAllowedError") {
            errorMessage += "Permission denied. Please allow access to your audio devices.";
        } else if (error.name === "NotFoundError") {
            errorMessage += "No audio device found. Please check your hardware connections.";
        } else if (error.name === "NotSupportedError") {
            const recordingMode = document.querySelector('input[name="recordingMode"]:checked').value;
            if (recordingMode === "system" || recordingMode === "both") {
                errorMessage += "System audio recording is not supported in this browser or configuration. Try using Chrome or Edge, or select 'Microphone only' mode.";
            } else {
                errorMessage += "The requested audio format is not supported in this browser.";
            }
        } else if (error.name === "NotReadableError") {
            errorMessage += "Could not access your audio device. It may be in use by another application.";
        } else if (error.name === "AbortError") {
            errorMessage += "The recording was aborted. Please try again.";
        } else if (error.name === "SecurityError") {
            errorMessage += "Security restriction prevented recording. Try using HTTPS.";
        } else {
            errorMessage += error.message || "Unknown error";
        }
        
        alert(errorMessage);
    }
}

function stopRecording() {
    document.getElementById("startBtn").disabled = false;
    document.getElementById("stopBtn").disabled = true;
    document.getElementById("status").classList.add("hidden");
    document.getElementById("audioVisualizer").classList.add("hidden");
    
    // Clear recording name input after recording is complete
    document.getElementById("recordingName").value = "";
    
    // Clear timers
    if (recordingTimer) clearInterval(recordingTimer);
    if (visualizerTimer) clearInterval(visualizerTimer);
    
    // Stop MediaRecorder if active
    if (mediaRecorder && mediaRecorder.state !== 'inactive') {
        mediaRecorder.stop();
    }
    
    // Disconnect and clean up audio processing
    if (processor) processor.disconnect();
    if (analyser) analyser.disconnect();
    
    // Stop all media tracks
    if (micStream) micStream.getTracks().forEach(track => track.stop());
    if (systemStream) systemStream.getTracks().forEach(track => track.stop());
    
    // Also stop tracks from the combined stream if it exists and is different
    if (finalStream && finalStream !== micStream && finalStream !== systemStream) {
        console.log("Stopping tracks from combined stream");
        finalStream.getTracks().forEach(track => track.stop());
    }
    
    // Close WebSocket connection with callback to ensure we fetch recordings after connection is closed
    if (socket) {
        // Make sure we listen for the close event BEFORE closing it
        socket.addEventListener('close', function(event) {
            console.log("WebSocket closed. Fetching updated recordings...");
            // Increase delay to ensure backend has time to process the recording
            setTimeout(fetchRecordings, 1500);
        }, { once: true }); // Use once:true to ensure it only fires once
        
        // Add a small delay before closing the socket
        setTimeout(() => {
            socket.close();
            socket = null;
        }, 500);
    } else {
        // If socket was never created, still update recordings
        setTimeout(fetchRecordings, 1500);
    }
    
    // Add an extra safeguard to ensure recordings are fetched
    setTimeout(fetchRecordings, 2000);
    
    // Reset stream references
    micStream = null;
    systemStream = null;
    combinedStream = null;
    mediaRecorder = null;
}