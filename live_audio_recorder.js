// Live Audio Recorder for Real-time Transcription
// Extracted and adapted from the main web UI audio recording functionality

class LiveAudioRecorder {
    constructor() {
        this.socket = null;
        this.micStream = null;
        this.audioContext = null;
        this.analyser = null;
        this.processor = null;
        this.isRecording = false;
        this.recordingStartTime = null;
        this.recordingTimer = null;
        this.visualizerTimer = null;
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
    async startRecording(wsUrl = null, languageCodes = []) {
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
            this.audioContext = new AudioContext({ sampleRate: 16000 }); // Google Speech-to-Text prefers 16kHz
            const audioStream = await this._setupAudioStreams();

            this._setupAudioContextAndVisualizer(audioStream);

            // Set up WebSocket connection
            if (wsUrl) {
                this._setupWebSocket(wsUrl, languageCodes);
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

            const errorEvent = new CustomEvent('recordererror', { detail: { message: 'Error starting recording: ' + error.message } });
            document.dispatchEvent(errorEvent);
            throw error; // Re-throw to propagate to UI
        }
    }

    async _setupAudioStreams() {
        const audioSelect = document.getElementById('audioSource');
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
        return this.micStream;
    }

    _setupAudioContextAndVisualizer(audioStream) {
        const source = this.audioContext.createMediaStreamSource(audioStream);
        this.analyser = this.audioContext.createAnalyser();
        this.analyser.fftSize = 256;
        source.connect(this.analyser);

        // Start visualizer updates
        this.visualizerTimer = setInterval(() => this.updateVisualizer(), 100);
    }

    _setupWebSocket(wsUrl, languageCodes) {
        this.socket = new WebSocket(wsUrl);
        this.socket.binaryType = "arraybuffer";

        this.socket.onopen = () => this._onWebSocketOpen(languageCodes);
        this.socket.onmessage = this._onWebSocketMessage.bind(this);
        this.socket.onerror = this._onWebSocketError.bind(this);
        this.socket.onclose = this._onWebSocketClose.bind(this);
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
            return;
        }

        const inputData = event.inputBuffer.getChannelData(0); // Raw PCM data
        const pcmData16 = this.convertFloat32ToInt16(inputData);
        console.log(`Sending audio data: ${pcmData16.byteLength} bytes`);
        // Send raw PCM data as binary message
        this.socket.send(pcmData16.buffer);
    }

    _onWebSocketOpen(languageCodes) {
        console.log("WebSocket connection established for Google Cloud Speech-to-Text live streaming");
        // Send initial configuration message with the summary prompt
        const configMessage = {
            type: "config",
            audioFormat: {
                format: "LINEAR16", // Google Speech-to-Text prefers LINEAR16
                sampleRate: this.audioContext.sampleRate,
                channels: 1 // Assuming mono audio
            },
            languageCodes: languageCodes // Include language codes
        };
        this.socket.send(JSON.stringify(configMessage));
        this.configSent = true;
        this.startAudioProcessing(this.micStream); // Pass the actual stream
    }

    _onWebSocketMessage(event) {
        try {
            const data = JSON.parse(event.data);
            console.log("Received from Google Cloud Speech-to-Text:", data);

            if (data.type === "summary") {
                const summaryEvent = new CustomEvent('summary', {
                    detail: data
                });
                document.dispatchEvent(summaryEvent);
            } else {
                // Assume it's a transcription update
                const transcriptionEvent = new CustomEvent('transcription', {
                    detail: data
                });
                document.dispatchEvent(transcriptionEvent);
            }
        } catch (error) {
            console.error("Error parsing server message:", error);
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
        }
        
        // Close WebSocket
        if (this.socket) {
            this.socket.close();
            this.socket = null;
        }
        
        // Reset references
        this.micStream = null;
        this.audioContext = null;
        this.analyser = null;
        
        console.log("Google Cloud Speech-to-Text live audio recording stopped");
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