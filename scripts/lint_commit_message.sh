#!/bin/bash

VALID_COMMIT_REGEX=""

ALLOWED_TYPES=("build" "chore" "ci" "docs" "feat" "fix" "perf" "refactor" "revert" "style" "test")
for TYPE in ${ALLOWED_TYPES[@]}; do
  VALID_COMMIT_REGEX="$VALID_COMMIT_REGEX$TYPE(\([a-zA-Z0-9 ]+\)){0,1}(!){0,1}: [a-z]|"
done

# Remove trailing pipe.
VALID_COMMIT_REGEX=$(echo $VALID_COMMIT_REGEX | rev | cut -c2- | rev)
# Add parens.
VALID_COMMIT_REGEX="(${VALID_COMMIT_REGEX})"


COMMIT_MESSAGE=$1
if [[ $COMMIT_MESSAGE =~ $VALID_COMMIT_REGEX ]]
then
  exit 0
else
  echo "Commit message does follow the conventional commit format."
  exit 1
fi
