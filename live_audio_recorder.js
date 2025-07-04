(function() {
    // Live Audio Recorder for Real-time Transcription
    // Extracted and adapted from the main web UI audio recording functionality

    if (typeof window.LiveAudioRecorder !== 'undefined') {
        // LiveAudioRecorder is already defined, skip re-declaration
        return;
    }

    class LiveAudioRecorder {
        constructor() {
            this.socket = null;
            this.micStream = null;
            this.systemStream = null;
            this.combinedStream = null;
            this.audioContext = null;
            this.analyser = null;
            this.processor = null;
            this.mediaRecorder = null;
            this.isRecording = false;
            this.recordingStartTime = null;
            this.recordingTimer = null;
            this.visualizerTimer = null;
        }

        // Check if the browser supports system audio capture
        isSystemAudioSupported() {
            // Check if getDisplayMedia is available
            if (!navigator.mediaDevices || !navigator.mediaDevices.getDisplayMedia) {
                console.warn("getDisplayMedia API not available");
                return false;
            }
            
            // Check browser compatibility
            const ua = navigator.userAgent.toLowerCase();
            
            // Safari currently doesn't support system audio capture
            if (ua.includes('safari') && !ua.includes('chrome') && !ua.includes('edg')) {
                console.warn("Safari detected - system audio not supported");
                return false;
            }
            
            // Check Chrome/Edge version
            if (ua.includes('chrome') || ua.includes('edg')) {
                const chromeMatch = ua.match(/chrom(?:e|ium)\/([0-9]+)/);
                if (chromeMatch && parseInt(chromeMatch[1]) < 74) {
                    console.warn("Chrome version < 74, system audio may not be supported");
                    return false;
                }
                return true;
            }
            
            // Firefox supports it but with limitations
            if (ua.includes('firefox')) {
                console.warn("Firefox has limited support for system audio capture");
                return true;
            }
            
            return false;
        }

        // Function to show/hide audio source dropdown based on recording mode
        updateAudioSourceVisibility() {
            const recordingMode = document.querySelector('input[name="recordingMode"]:checked')?.value || 'microphone';
            const audioSourceContainer = document.querySelector('.audio-source-selector');
            
            if (recordingMode === 'microphone' || recordingMode === 'both') {
                if (audioSourceContainer) audioSourceContainer.style.display = 'block';
            } else {
                if (audioSourceContainer) audioSourceContainer.style.display = 'none';
            }
            
            // Show browser compatibility warning if needed
            const systemAudioSupported = this.isSystemAudioSupported();
            if ((recordingMode === 'system' || recordingMode === 'both') && !systemAudioSupported) {
                const warningEl = document.getElementById('browserWarning');
                if (!warningEl) {
                    const warning = document.createElement('div');
                    warning.id = 'browserWarning';
                    warning.className = 'browser-warning';
                    warning.innerHTML = `
                        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16">
                            <path d="M8 15A7 7 0 1 1 8 1a7 7 0 0 1 0 14zm0 1A8 8 0 1 0 8 0a8 8 0 0 0 0 16z"/>
                            <path d="M7.002 11a1 1 0 1 1 2 0 1 1 0 0 1-2 0zM7.1 4.995a.905.905 0 1 1 1.8 0l-.35 3.507a.552.552 0 0 1-1.1 0L7.1 4.995z"/>
                        </svg>
                        <span>System audio recording is not supported in this browser. Please use Chrome or Edge for this feature.</span>
                    `;
                    const controlPanel = document.querySelector('.control-panel');
                    if (controlPanel) controlPanel.appendChild(warning);
                }
            } else {
                const warningEl = document.getElementById('browserWarning');
                if (warningEl) {
                    warningEl.remove();
                }
            }
        }

        // Populate audio input devices dropdown
        async populateAudioDevices() {
            const audioSelect = document.getElementById('audioSource');
            
            try {
                // Get permission to access media devices
                const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
                stream.getTracks().forEach(track => track.stop());
                
                // Enumerate devices
                const devices = await navigator.mediaDevices.enumerateDevices();
                
                // Clear previous options
                audioSelect.innerHTML = '';
                
                // Add default option
                const defaultOption = document.createElement('option');
                defaultOption.value = 'default';
                defaultOption.text = 'Default Microphone';
                defaultOption.selected = true;
                audioSelect.appendChild(defaultOption);
                
                // Add audio input devices
                const audioInputDevices = devices.filter(device => device.kind === 'audioinput');
                audioInputDevices.forEach(device => {
                    const option = document.createElement('option');
                    option.value = device.deviceId;
                    option.text = device.label || `Microphone ${audioSelect.length + 1}`;
                    audioSelect.appendChild(option);
                });
                
                console.log(`Found ${audioInputDevices.length} audio input devices`);
                
                // Set up recording mode change listeners
                document.querySelectorAll('input[name="recordingMode"]').forEach(radio => {
                    radio.addEventListener('change', () => this.updateAudioSourceVisibility());
                });
                
                // Initialize visibility
                this.updateAudioSourceVisibility();
                
            } catch (err) {
                console.error('Error enumerating audio devices:', err);
                const errorEvent = new CustomEvent('recordererror', { detail: { message: 'Error enumerating audio devices: ' + err.message } });
                document.dispatchEvent(errorEvent);
                
                // Add fallback option
                const fallbackOption = document.createElement('option');
                fallbackOption.value = 'default';
                fallbackOption.text = 'Default Microphone';
                fallbackOption.selected = true;
                audioSelect.appendChild(fallbackOption);
            }
        }

        // Initialize visualizer bars
        initVisualizer() {
            const visualizerBars = document.getElementById('visualizerBars');
            if (!visualizerBars) return;
            
            visualizerBars.innerHTML = '';
            
            // Create 30 bars for the visualizer
            for (let i = 0; i < 30; i++) {
                const bar = document.createElement('div');
                bar.className = 'visualizer-bar';
                visualizerBars.appendChild(bar);
            }
        }

        // Update visualizer with current audio data
        updateVisualizer() {
            if (!this.analyser) return;
            
            const bufferLength = this.analyser.frequencyBinCount;
            const dataArray = new Uint8Array(bufferLength);
            this.analyser.getByteFrequencyData(dataArray);
            
            const bars = document.querySelectorAll('.visualizer-bar');
            const step = Math.floor(bufferLength / bars.length);
            
            for (let i = 0; i < bars.length; i++) {
                const value = dataArray[i * step];
                const height = Math.max(5, value / 255 * 100);
                bars[i].style.height = `${height}%`;
            }
        }

        // Update recording time counter
        updateRecordingTime() {
            const elapsed = Math.floor((Date.now() - this.recordingStartTime) / 1000);
            const minutes = Math.floor(elapsed / 60).toString().padStart(2, '0');
            const seconds = (elapsed % 60).toString().padStart(2, '0');
            const timeElement = document.getElementById('recordingTime');
            if (timeElement) {
                timeElement.textContent = `${minutes}:${seconds}`;
            }
        }

        // Start recording audio stream for Google Cloud Speech-to-Text live transcription
        async startRecording(wsUrl = null, languageCodes = [], customWords = [], phraseSetsConfig = null, classesConfig = null) {
            this.configSent = false; // Flag to ensure config is sent first
            if (this.isRecording) {
                console.warn('Recording already in progress');
                return;
            }

            this.isRecording = true;
            this.recordingStartTime = Date.now();
            this.recordingTimer = setInterval(() => this.updateRecordingTime(), 1000);

            // Initialize visualizer
            this.initVisualizer();

            try {
                this.audioContext = new AudioContext(); // Use browser default sample rate for compatibility
                const audioStream = await this._setupAudioStreams();

                this._setupAudioContextAndVisualizer(audioStream);

                // Set up WebSocket connection
                if (wsUrl) {
                    this._setupWebSocket(wsUrl, languageCodes, customWords, phraseSetsConfig, classesConfig);
                } else {
                    this.startAudioProcessing(audioStream);
                }

                console.log(`Live audio recording started.`);
            } catch (error) {
                console.error("Error starting recording:", error);
                this.isRecording = false;

                // Clean up on error
                if (this.recordingTimer) clearInterval(this.recordingTimer);
                if (this.visualizerTimer) clearInterval(this.visualizerTimer);
                if (this.micStream) {
                    this.micStream.getTracks().forEach(track => track.stop());
                    this.micStream = null;
                }
                if (this.systemStream) {
                    this.systemStream.getTracks().forEach(track => track.stop());
                    this.systemStream = null;
                }

                const errorEvent = new CustomEvent('recordererror', { detail: { message: 'Error starting recording: ' + error.message } });
                document.dispatchEvent(errorEvent);
                throw error; // Re-throw to propagate to UI
            }
        }

        async _setupAudioStreams() {
            const recordingMode = document.querySelector('input[name="recordingMode"]:checked')?.value || 'microphone';
            const audioSelect = document.getElementById('audioSource');
            
            let finalStream;
            
            // Handle microphone audio
            if (recordingMode === 'microphone' || recordingMode === 'both') {
                const micConstraints = {
                    audio: {
                        deviceId: audioSelect?.value !== 'default' ? { exact: audioSelect.value } : undefined,
                        sampleRate: this.audioContext.sampleRate,
                        channelCount: 1,
                        echoCancellation: true,
                        noiseSuppression: true,
                        autoGainControl: true
                    }
                };
                this.micStream = await navigator.mediaDevices.getUserMedia(micConstraints);
                if (!this.micStream || this.micStream.getAudioTracks().length === 0) {
                    throw new Error("No microphone audio tracks available.");
                }
                finalStream = this.micStream;
            }
            
            // Handle system audio
            if (recordingMode === 'system' || recordingMode === 'both') {
                if (!this.isSystemAudioSupported()) {
                    if (recordingMode === 'system') {
                        throw new Error("Your browser doesn't support system audio capture. Try using Chrome or Edge browser.");
                    } else {
                        console.warn("System audio capture is not supported in your browser. Recording microphone only.");
                        return finalStream || this.micStream;
                    }
                }
                
                try {
                    // Request system audio (this will prompt the user to share their screen/audio)
                    this.systemStream = await navigator.mediaDevices.getDisplayMedia({ 
                        video: true,  // Need video: true for many browsers
                        audio: true
                    });
                    
                    // Check if user actually shared audio
                    const hasAudio = this.systemStream.getAudioTracks().length > 0;
                    if (!hasAudio) {
                        if (recordingMode === 'system') {
                            throw new Error("No system audio was shared. Make sure to select 'Share audio' when prompted.");
                        } else {
                            console.warn("No system audio was shared. Recording microphone only.");
                            return finalStream || this.micStream;
                        }
                    }
                    
                    console.log('System audio capture enabled');
                    console.log('System audio tracks:', this.systemStream.getAudioTracks().length);
                    
                    // Turn off video tracks as we only need audio
                    this.systemStream.getVideoTracks().forEach(track => {
                        track.enabled = false;
                    });
                    
                    // Ensure audio tracks remain enabled
                    this.systemStream.getAudioTracks().forEach(track => {
                        track.enabled = true;
                    });
                    
                    if (recordingMode === 'system') {
                        finalStream = this.systemStream;
                    } else if (recordingMode === 'both' && this.micStream) {
                        // Combine streams
                        finalStream = await this._combineAudioStreams(this.micStream, this.systemStream);
                        this.combinedStream = finalStream; // Store the combined stream
                    } else {
                        finalStream = this.systemStream;
                    }
                    
                } catch (err) {
                    console.error("Error getting system audio:", err);
                    if (recordingMode === 'system') {
                        if (err.name === 'NotAllowedError') {
                            throw new Error("Permission denied. Please allow screen sharing with audio.");
                        } else if (err.name === 'NotSupportedError') {
                            throw new Error("System audio capture is not supported in your browser configuration.");
                        } else {
                            throw err;
                        }
                    } else {
                        console.warn("Could not capture system audio. Recording microphone only.");
                        return finalStream || this.micStream;
                    }
                }
            }
            
            return finalStream;
        }
        
        async _combineAudioStreams(micStream, systemStream) {
            try {
                console.log("Mixing microphone and system audio streams");
                
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
                const finalStream = dest.stream;
                console.log("Created mixed audio stream with tracks:", finalStream.getAudioTracks().length);
                
                return finalStream;
                
            } catch (error) {
                console.error("Error mixing audio streams:", error);
                // Fallback to just microphone if mixing fails
                return micStream;
            }
        }

        _setupAudioContextAndVisualizer(audioStream) {
            const source = this.audioContext.createMediaStreamSource(audioStream);
            this.analyser = this.audioContext.createAnalyser();
            this.analyser.fftSize = 256;
            source.connect(this.analyser);

            // Start visualizer updates
            this.visualizerTimer = setInterval(() => this.updateVisualizer(), 100);
        }

        _setupWebSocket(wsUrl, languageCodes, customWords, phraseSetsConfig, classesConfig) {
            this.socket = new WebSocket(wsUrl);
            this.socket.binaryType = "arraybuffer";

            this.socket.onopen = () => this._onWebSocketOpen(languageCodes, customWords, phraseSetsConfig, classesConfig);
            this.socket.onmessage = this._onWebSocketMessage.bind(this);
            this.socket.onerror = this._onWebSocketError.bind(this);
            this.socket.onclose = this._onWebSocketClose.bind(this);
        }

        _setupMediaRecorder(languageCodes) {
            const recordingMode = document.querySelector('input[name="recordingMode"]:checked')?.value || 'microphone';
            let streamToRecord;
            
            if (recordingMode === 'system') {
                // Create a clean stream with just the system audio track (like in working sample)
                if (this.systemStream && this.systemStream.getAudioTracks().length > 0) {
                    const audioTrack = this.systemStream.getAudioTracks()[0];
                    streamToRecord = new MediaStream([audioTrack]);
                    console.log("Created clean system audio stream");
                }
            } else if (recordingMode === 'both') {
                // For 'both' mode, we need to use the combined stream
                // The combined stream should be stored in a property after mixing
                streamToRecord = this.combinedStream || this.systemStream;
            } else {
                streamToRecord = this.micStream;
            }
            
            if (!streamToRecord) {
                console.error("No stream available for MediaRecorder");
                return;
            }
            
            // Validate stream has active audio tracks
            const audioTracks = streamToRecord.getAudioTracks();
            console.log(`Stream has ${audioTracks.length} audio tracks`);
            
            if (audioTracks.length === 0) {
                console.error("Stream has no audio tracks");
                return;
            }
            
            // Check if tracks are active
            const activeTracks = audioTracks.filter(track => track.readyState === 'live');
            console.log(`${activeTracks.length} audio tracks are active`);
            
            if (activeTracks.length === 0) {
                console.error("No active audio tracks in stream");
                return;
            }
            
            // Use the same format as microphone recording for consistency
            let actualFormat = "LINEAR16";
            let actualSampleRate = this.audioContext.sampleRate;
            
            // Use AudioContext approach for consistent processing
            console.log("Setting up system audio processing with AudioContext");
            this._setupSystemAudioWithAudioContext(streamToRecord, languageCodes);
        }

        _setupSystemAudioWithAudioContext(systemStream, languageCodes) {
            console.log("Setting up system audio processing with AudioContext");
            
            // Create audio source from system stream
            const source = this.audioContext.createMediaStreamSource(systemStream);
            
            // Create script processor for real-time audio processing (same as microphone)
            this.processor = this.audioContext.createScriptProcessor(1024, 1, 1);
            this.processor.onaudioprocess = this._onAudioProcess.bind(this);
            
            // Connect the processing chain
            source.connect(this.processor);
            this.processor.connect(this.audioContext.destination);
            
            console.log("System audio processing setup complete - using PCM data streaming");
        }

        // Start audio processing for Google Cloud Speech-to-Text live streaming
        startAudioProcessing(stream) {
            if (!this.audioContext || !stream) {
                console.error("Audio context or stream not available");
                return;
            }

            const source = this.audioContext.createMediaStreamSource(stream);

            // Create script processor for real-time audio processing
            this.processor = this.audioContext.createScriptProcessor(1024, 1, 1);
            this.processor.onaudioprocess = this._onAudioProcess.bind(this);

            source.connect(this.processor);
            this.processor.connect(this.audioContext.destination);

            console.log("Audio processing started for Google Cloud Speech-to-Text");
        }

        _onAudioProcess(event) {
            if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
                console.log("⚠️ Cannot send audio: WebSocket not ready");
                return;
            }

            const inputData = event.inputBuffer.getChannelData(0); // Raw PCM data
            const pcmData16 = this.convertFloat32ToInt16(inputData);
            // Reduced logging: only log occasionally to avoid spam
            if (Math.random() < 0.001) { // Log 0.1% of the time
                console.log(`🎵 Audio data: ${pcmData16.byteLength} bytes`);
            }
            
            // Update waveform visualization if available
            if (window.updateWaveform && typeof window.updateWaveform === 'function') {
                window.updateWaveform(inputData);
            }
            
            // Send raw PCM data as binary message
            this.socket.send(pcmData16.buffer);
        }

        _onWebSocketOpen(languageCodes, customWords = [], phraseSetsConfig = null, classesConfig = null) {
            console.log("🔗 WebSocket connection established for Google Cloud Speech-to-Text live streaming");
            
            const recordingMode = document.querySelector('input[name="recordingMode"]:checked')?.value || 'microphone';
            
            // Get current custom prompt from the frontend
            const customPrompt = window.getCurrentSummaryPrompt ? window.getCurrentSummaryPrompt() : '';
            
            // Send config message for all modes
            const configMessage = {
                type: "config",
                audioFormat: {
                    format: "LINEAR16",
                    sampleRate: this.audioContext.sampleRate,
                    channels: 1
                },
                languageCode: languageCodes.length > 0 ? languageCodes[0] : "en-US",
                alternativeLanguageCodes: languageCodes.slice(1),
                customWords: customWords || [],
                phraseSets: phraseSetsConfig,
                classes: classesConfig,
                summaryPrompt: customPrompt
            };
            console.log("📤 Sending config message:", configMessage);
            this.socket.send(JSON.stringify(configMessage));
            this.configSent = true;
            
            // For system audio or mixed mode, use MediaRecorder approach
            if (recordingMode === 'system' || recordingMode === 'both') {
                this._setupMediaRecorder(languageCodes);
            } else {
                // For microphone only, use the original PCM approach
                this.startAudioProcessing(this.micStream);
            }
        }

        _onWebSocketMessage(event) {
            try {
                const data = JSON.parse(event.data);
                // Reduced logging: only log message type, not full content
                console.log("📦 Message type:", data.type);

                if (data.type === "summary") {
                    const summaryEvent = new CustomEvent('summary', {
                        detail: data
                    });
                    document.dispatchEvent(summaryEvent);
                } else if (data.type === "transcription") {
                    const transcriptionEvent = new CustomEvent('transcription', {
                        detail: data
                    });
                    document.dispatchEvent(transcriptionEvent);
                } else if (data.type === "status") {
                    if (data.status !== 'stream_recreated') { // Only log non-routine status updates
                        console.log("📊 Status:", data.status, data.message);
                    }
                    const statusEvent = new CustomEvent('recorderstatus', {
                        detail: data
                    });
                    document.dispatchEvent(statusEvent);
                } else {
                    console.warn("⚠️ Unknown message type:", data.type);
                }
            } catch (error) {
                console.error("❌ Error parsing server message:", error, "Raw data:", event.data);
            }
        }

        _onWebSocketError(error) {
            console.error("WebSocket error:", error);
            this.stopRecording();
            const errorEvent = new CustomEvent('recordererror', { detail: { message: 'WebSocket error: ' + error.message } });
            document.dispatchEvent(errorEvent);
        }

        _onWebSocketClose(event) {
            console.log("WebSocket connection closed", event);
            // Dispatch an event to indicate WebSocket closure
            const closeEvent = new CustomEvent('recorderclosed', { detail: { code: event.code, reason: event.reason } });
            document.dispatchEvent(closeEvent);
        }

        // Stop recording
        stopRecording() {
            if (!this.isRecording) {
                console.warn('No recording in progress');
                return;
            }

            this.isRecording = false;
            
            // Clear timers
            if (this.recordingTimer) clearInterval(this.recordingTimer);
            if (this.visualizerTimer) clearInterval(this.visualizerTimer);
            
            // Stop audio processor
            if (this.processor) {
                this.processor.disconnect();
                this.processor = null;
            }
            
            // Stop MediaRecorder
            if (this.mediaRecorder && this.mediaRecorder.state !== 'inactive') {
                this.mediaRecorder.stop();
            }
            
            // Stop media tracks
            if (this.micStream) {
                this.micStream.getTracks().forEach(track => track.stop());
                this.micStream = null;
            }
            
            if (this.systemStream) {
                this.systemStream.getTracks().forEach(track => track.stop());
                this.systemStream = null;
            }
            
            if (this.combinedStream) {
                this.combinedStream.getTracks().forEach(track => track.stop());
                this.combinedStream = null;
            }
            
            // Don't immediately close WebSocket if we're waiting for final summary
            if (window.waitingForFinalSummary) {
                console.log("Keeping WebSocket open for final summary");
                // Set a timeout to close the WebSocket if final summary takes too long
                setTimeout(() => {
                    if (this.socket) {
                        console.log("Closing WebSocket after final summary timeout");
                        this.socket.close();
                        this.socket = null;
                    }
                }, 15000); // 15 seconds max wait
            } else {
                // Close WebSocket immediately if not waiting for final summary
                if (this.socket) {
                    this.socket.close();
                    this.socket = null;
                }
            }
            
            // Reset references
            this.audioContext = null;
            this.analyser = null;
            this.mediaRecorder = null;
            
            console.log("Google Cloud Speech-to-Text live audio recording stopped");
        }

        // Method to force close WebSocket (called after final summary is received)
        forceCloseWebSocket() {
            if (this.socket) {
                console.log("Force closing WebSocket after final summary received");
                this.socket.close();
                this.socket = null;
            }
        }

        // Get current recording state
        getRecordingState() {
            return {
                isRecording: this.isRecording,
                recordingTime: this.recordingStartTime ? Math.floor((Date.now() - this.recordingStartTime) / 1000) : 0
            };
        }
        
        // Convert Float32 audio data to Int16 PCM
        convertFloat32ToInt16(float32Array) {
            const int16Array = new Int16Array(float32Array.length);
            for (let i = 0; i < float32Array.length; i++) {
                int16Array[i] = Math.max(-32768, Math.min(32767, float32Array[i] * 32768));
            }
            return int16Array;
        }
    }

    // Export for use in other modules
    if (typeof module !== 'undefined' && module.exports) {
        module.exports = LiveAudioRecorder;
    } else {
        window.LiveAudioRecorder = LiveAudioRecorder;
    }
})();