// Global variables
let socket;
let audioContext;
let processor;
let micStream;
let systemStream;
let combinedStream;
let analyser;
let recordingStartTime;
let recordingTimer;
let visualizerTimer;
let mediaRecorder;

// Initialize when page loads
window.addEventListener('load', () => {
    fetchRecordings();
    loadThemePreference();
    
    // Initialize visualizer container
    initVisualizer();
    
    // Populate audio devices dropdown
    populateAudioDevices();
    
    // Listen for device changes (e.g., if user connects a new microphone)
    navigator.mediaDevices.addEventListener('devicechange', populateAudioDevices);
    
    // Add event listeners for recording mode radio buttons
    document.querySelectorAll('input[name="recordingMode"]').forEach(radio => {
        radio.addEventListener('change', updateAudioSourceVisibility);
    });
});

// Close modal with Escape key
document.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        closePromptModal();
        closeContentModal();
    }
});

// Toggle dark/light theme
function toggleTheme() {
    document.body.classList.toggle('dark-mode');
    localStorage.setItem('darkMode', document.body.classList.contains('dark-mode'));
}

// Check for saved theme preference
function loadThemePreference() {
    if (localStorage.getItem('darkMode') === 'true') {
        document.body.classList.add('dark-mode');
    }
}

// Format recording date from filename
function formatRecordingDate(filename) {
    // Assuming filename format contains date/time information
    // Modify this function based on your actual filename format
    try {
        // Example: recording_2023-03-01_15-30-45.wav
        const parts = filename.split('_');
        if (parts.length >= 3) {
            const datePart = parts[1];
            const timePart = parts[2].split('.')[0];
            return `${datePart.replace(/-/g, '/')} at ${timePart.replace(/-/g, ':')}`;
        }
        return filename;
    } catch (e) {
        return filename;
    }
}

// Check if the browser supports AudioWorklet
function isAudioWorkletSupported() {
    return window.AudioContext && 'audioWorklet' in (new AudioContext());
}

// Check if the browser supports system audio capture
function isSystemAudioSupported() {
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
    
    // Check Chrome/Edge version (system audio supported in Chrome ≥ 74, Edge on Chromium)
    if (ua.includes('chrome') || ua.includes('edg')) {
        // Extract Chrome version
        const chromeMatch = ua.match(/chrom(?:e|ium)\/([0-9]+)/);
        if (chromeMatch && parseInt(chromeMatch[1]) < 74) {
            console.warn("Chrome version < 74, system audio may not be supported");
            return false;
        }
        
        // Chrome/Edge with recent versions should support it
        console.log("Chrome/Edge detected, system audio should be supported");
        return true;
    }
    
    // Firefox supports it but with limitations
    if (ua.includes('firefox')) {
        console.warn("Firefox has limited support for system audio capture");
        // Firefox supports it after version 66, but with varying reliability
        return true;
    }
    
    // Default to false for unknown browsers
    console.warn("Unknown browser, assuming no system audio support");
    return false;
}

// Function to show/hide audio source dropdown based on recording mode
function updateAudioSourceVisibility() {
    const recordingMode = document.querySelector('input[name="recordingMode"]:checked').value;
    const audioSourceContainer = document.querySelector('.audio-source-selector:first-child');
    
    if (recordingMode === 'microphone' || recordingMode === 'both') {
        audioSourceContainer.style.display = 'block';
    } else {
        audioSourceContainer.style.display = 'none';
    }
    
    // Show browser compatibility warning if needed
    const systemAudioSupported = isSystemAudioSupported();
    if ((recordingMode === 'system' || recordingMode === 'both') && !systemAudioSupported) {
        // Add a warning message
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
            document.querySelector('.control-panel').appendChild(warning);
        }
    } else {
        // Remove warning if it exists and not needed
        const warningEl = document.getElementById('browserWarning');
        if (warningEl) {
            warningEl.remove();
        }
    }
}