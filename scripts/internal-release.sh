#!/bin/bash

set -e 
set -o pipefail

BOLDGREEN="\033[32m"
ENDCOLOR="\033[0m"

REGISTRY_URL=694773929020.dkr.ecr.us-east-1.amazonaws.com

LogStep() {
  echo -e "${BOLDGREEN}##\n$1\n##${ENDCOLOR}"
}

Help()
{
   echo "This script builds and pushes the Docker image to ECR."
   echo
   echo "Usage: ./release.sh $COMMIT_HASH"
   echo
}

COMMIT_HASH=$1

if [ -z "$COMMIT_HASH" ]
then
  Help
  exit 0
fi

VALID_LONG_GIT_SHA_REGEX="[a-f0-9]{40}"
if [[ ! $COMMIT_HASH =~ $VALID_LONG_GIT_SHA_REGEX ]]
then
 echo "Invalid long commit hash."
  exit 1
fi

if [ "$(git status --porcelain)" ]
then 
  echo "The working directory must be clean to perform a release."
  exit 1
fi

LogStep "Docker login."
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin 694773929020.dkr.ecr.us-east-1.amazonaws.com

LogStep "Releasing commit $COMMIT_HASH."

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

LogStep "Checking out the commit to release."
git checkout $COMMIT_HASH

LogStep "Running code linter."
golangci-lint run

LogStep "Running tests."
go test -v ./...

LogStep "Cleaning the build directory."
make clean

LogStep "Building and pushing the image."
make GIT_COMMIT_HASH=$COMMIT_HASH VERSION=$COMMIT_HASH DOCKER_HUB_REPO=$REGISTRY_URL/node-gateway build-and-push-image

LogStep "Checking out the original commit."
git checkout $CURRENT_BRANCH
