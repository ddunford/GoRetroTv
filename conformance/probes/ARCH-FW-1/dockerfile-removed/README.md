# Move the image build definition aside

Moves Dockerfile away. The coverage detector must report `firmware-subject-missing`, rather than
claiming an image check passed without a build definition.
