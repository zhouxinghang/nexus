# Journal entries are append-only and immutable

Journal Entries cannot be edited or deleted once written; each is plain text
with an automatic timestamp. This deliberately departs from the usual
assumption that every record supports full CRUD. The Journal is a chronological
stream of what happened — an immutable log — so allowing edits or deletes would
undermine its purpose as a trustworthy record. Resources, Tasks, and other
entities remain fully mutable; this constraint is specific to the Journal.
