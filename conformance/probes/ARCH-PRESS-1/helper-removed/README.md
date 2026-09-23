# Missing press helper

Moves the file that owns `pressAndLetItFinishWatching` away. `ARCH-PRESS-1` must report
`press-subject-missing`, because a corpus with no settle loop anywhere passes the import check
having examined nothing — and it is the helper moving in a refactor, not a decision, that gets it
there.
