package imageproviders

import "time"

// requestTimeout bounds a whole request to an image provider: the result is read in full, and a generation can take minutes.
// Without it a provider that stops answering would hold the call forever.
const requestTimeout = 5 * time.Minute
