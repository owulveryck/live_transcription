/**
 * AudioWorkletProcessor for real-time audio processing
 * Replaces the deprecated ScriptProcessorNode
 */
class AudioProcessor extends AudioWorkletProcessor {
    constructor() {
        super();
        this.bufferSize = 1024; // Same buffer size as original ScriptProcessorNode
        this.audioBuffer = new Float32Array(this.bufferSize);
        this.bufferIndex = 0;
    }

    process(inputs, outputs, parameters) {
        const input = inputs[0];
        const output = outputs[0];

        if (input && input.length > 0) {
            const inputChannel = input[0]; // Get first channel (mono)
            
            // Don't copy input to output to avoid audio feedback
            // We only need to process the audio for transcription
            
            // Accumulate audio data in buffer
            for (let i = 0; i < inputChannel.length; i++) {
                this.audioBuffer[this.bufferIndex] = inputChannel[i];
                this.bufferIndex++;

                // When buffer is full, send to main thread
                if (this.bufferIndex >= this.bufferSize) {
                    // Convert Float32 to Int16 for speech API
                    const pcmData16 = this.convertFloat32ToInt16(this.audioBuffer);
                    
                    // Send audio data to main thread
                    this.port.postMessage({
                        type: 'audio-data',
                        data: pcmData16,
                        rawData: new Float32Array(this.audioBuffer) // For waveform visualization
                    });

                    // Reset buffer
                    this.bufferIndex = 0;
                }
            }
        }

        // Keep processor alive
        return true;
    }

    convertFloat32ToInt16(float32Array) {
        const int16Array = new Int16Array(float32Array.length);
        for (let i = 0; i < float32Array.length; i++) {
            // Clamp to [-1, 1] and convert to 16-bit PCM
            const clampedValue = Math.max(-1, Math.min(1, float32Array[i]));
            int16Array[i] = clampedValue * 32767;
        }
        return int16Array;
    }
}

registerProcessor('audio-processor', AudioProcessor);