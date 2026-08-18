# Adapter fixtures

These are **synthetic** approximations of each board's apply form, not captures
from the live sites: djinni, DOU and work.ua all put the apply form behind a
login, so it cannot be captured without an account.

They exercise the adapter contract — form closed vs open, hidden file inputs,
login walls, renamed classes — which is what the unit tests are for. They do
**not** prove the primary selectors match production markup. Replace each file
with a real capture (`document.documentElement.outerHTML` of the apply area,
scrubbed of personal data) the first time the extension is driven against a
live vacancy, and keep the churn fixtures alongside.
