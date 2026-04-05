# SO-79: Favicon Fix

## Problem
The app favicon was reported as missing or not loading correctly. The previous implementation used a custom handler for `/favicon.svg` which could be bypassed or fail due to browser preferences (e.g., requesting `/favicon.ico` or using query strings for cache busting).

## Solution
1. **Updated Favicon Asset**: The `static/favicon.svg` was updated with the new brand-compliant SVG mark (Amber-gold mark on near-black stone) as specified in the brand-system design document.
2. **Standardized Link**: Updated `internal/templates/partials.html` to point to `/static/favicon.svg` instead of the root `/favicon.svg`. This ensures it's served by the standard static file server under the `/static/` prefix.
3. **Compatibility Handlers**:
    - Retained `/favicon.svg` root handler in `main.go` for backward compatibility, using the more robust `fs.ReadFile(staticSub, ...)` approach.
    - Added `/favicon.ico` handler in `main.go` that redirects to `/static/favicon.svg` to satisfy browsers that request it by default.

## Verification
- Verified build compatibility with Go 1.26 features.
- Local tests confirmed that the favicon handler returns the correct content type and SVG data.
