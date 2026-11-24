---
name: changelog
description: Writes a CHANGELOG entry from the commits since the last tag.
---
# Changelog

1. List the commits since the last tag with `git log --oneline $(git describe --tags --abbrev=0)..HEAD`.
2. Group them by type: features, fixes, other.
3. Add a section at the top of CHANGELOG.md with today's date and the groups,
   one line per change, in plain words.
4. Show the user the new section before saving it.
