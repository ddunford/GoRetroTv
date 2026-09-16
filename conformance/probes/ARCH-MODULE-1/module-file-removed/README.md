# Move go.mod away

Moves the tracked module declaration away. The rule must report `module-file-missing`, since a
green dependency check cannot be based on a missing module file.
