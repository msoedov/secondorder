# Confirmation Interaction Patterns

## Destructive Actions

For destructive actions (complete/cancel work blocks, cancel issues), the system provides a standard browser-based confirmation dialog. This ensures that accidental clicks do not lead to non-reversible state changes.

### Pattern: Native Confirm

For terminal or destructive actions where a custom UI is not yet warranted, use the native `onclick="return confirm('...')"` on the action button.

Example (Work Block Detail):
```html
<form method="POST" action="/work-blocks/{{.Block.ID}}">
  <input type="hidden" name="action" value="ship">
  <button type="submit" onclick="return confirm('Are you sure you want to ship this work block?')">
    Ship
  </button>
</form>
```

### Standard Messages

- **Ship Work Block:** `Are you sure you want to ship this work block?`
- **Cancel Work Block:** `Are you sure you want to cancel this work block?`
- **Cancel Issue:** `Are you sure you want to cancel this issue?`

### Preferred Implementation

- **Location:** On the `<button type="submit">`.
- **Method:** `onclick="return confirm('...')"` to intercept the click event before form submission.
