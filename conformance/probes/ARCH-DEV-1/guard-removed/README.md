# Remove the non-loopback refusal

Disables the real config loader's refusal branch. The bind checker must observe public bind
addresses being accepted without the container override and report `non-loopback-accepted`.
