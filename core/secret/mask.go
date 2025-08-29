package secret

import "strings"

// Mask returns a masked representation of a secret string.
// - length <= 5: fully masked
// - length <= 20: first and last characters visible
// - length > 20: first 3 and last 1 characters visible
func Mask(s string) string {
    n := len(s)
    if n == 0 {
        return ""
    }
    if n <= 5 {
        return strings.Repeat("*", n)
    }
    if n <= 20 {
        // first and last characters visible
        if n == 1 { // paranoia, though n>5 here
            return "*"
        }
        return s[:1] + strings.Repeat("*", n-2) + s[n-1:]
    }
    // long secrets: first 3 and last 1 visible
    if n <= 4 { // defensive
        return strings.Repeat("*", n)
    }
    return s[:3] + strings.Repeat("*", n-4) + s[n-1:]
}

