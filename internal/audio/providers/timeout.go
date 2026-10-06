package audioproviders

import "time"

// requestTimeout bounds a whole request to a speech provider: the audio is sent and the result read in full, and a transcription
// of a long recording can take minutes. Without it a provider that stops answering would hold the call forever.
const requestTimeout = 5 * time.Minute
