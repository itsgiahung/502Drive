# U12 — Desktop needs-action notices

Tray compares observed job/account transitions. Blocked/quota/auth reasons use the
shared explanation catalogue, coalesced per cause for 60 seconds. A first snapshot
only primes the baseline. Notification preferences and quiet hours are checked;
unknown settings suppress delivery. Linux uses the existing D-Bus dependency for
Freedesktop notifications with a fixed `open` action. The callback captures the job
ID, never user text as an action. A bounded signal listener opens the job through
the U13 hook. notify-send remains the non-action fallback. No engine or API changes.
Windows native notifications are not present in the existing backend; this slice
does not claim Windows delivery. Both target builds still must compile.
