package slackauth

// isLeaklessEnabled is false on Windows because it was causing a false
// positive, see #260.  Windows is always so fucking special.
const isLeaklessEnabled = false
