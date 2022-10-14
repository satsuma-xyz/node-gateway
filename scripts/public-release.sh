#!/bin/bash
# This script releases the image to our public DockerHub repository.

set -e 
set -o pipefail

BOLDGREEN="\033[32m"
ENDCOLOR="\033[0m"

PRODUCTION_DOCKER_HUB_REPO="satsumaxyz/node-gateway"
TEST_DOCKER_HUB_REPO="dianwen/satsuma-gateway-test"

LogStep() {
  echo -e "${BOLDGREEN}##\n$1\n##${ENDCOLOR}"
}

Help()
{
   echo "This script builds and pushes the Docker image to a Docker Hub repository and generates source code archives for the GitHub release."
   echo "It can be used to push to the $PRODUCTION_DOCKER_HUB_REPO production repo or the $TEST_DOCKER_HUB_REPO test repo."
   echo
   echo "Usage: ./release.sh $COMMIT_HASH $RELEASE_VERSION $DOCKER_HUB_REPO"
   echo
}

IsProductionRelease()
{
  [ "$DOCKER_HUB_REPO" == "$PRODUCTION_DOCKER_HUB_REPO" ]
}

COMMIT_HASH=$1
RELEASE_VERSION=$2
DOCKER_HUB_REPO=$3

if [ -z "$COMMIT_HASH" ] || [ -z "$RELEASE_VERSION" ]
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

VALID_VERSION_REGEX="^[0-9]+\.[0-9]+\.[0-9]+$"
if [[ ! $RELEASE_VERSION =~ $VALID_VERSION_REGEX ]]
then
  echo "Invalid version. Expecting a semvar version like \`2.0.0\`."
  exit 1
fi

if [ ! "$DOCKER_HUB_REPO" == "$TEST_DOCKER_HUB_REPO" ] && [ ! "$DOCKER_HUB_REPO" == "$PRODUCTION_DOCKER_HUB_REPO" ]
then
  echo "Invalid Docker Hub repo. Expecting either $TEST_DOCKER_HUB_REPO or $PRODUCTION_DOCKER_HUB_REPO."
  exit 1
fi

# if [ "$(git status --porcelain)" ]
# then 
#   echo "The working directory must be clean to perform a release."
#   exit 1
# fi

if IsProductionRelease
then
  read -r -p "Have you already tested this image by pushing it up to the $PRODUCTION_DOCKER_HUB_REPO Docker registry? [y/N] " response
  if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]
  then
      exit 1
  fi
fi

LogStep "Log in to the dianwen Docker account."
docker login -u dianwen

LogStep "Releasing version $RELEASE_VERSION using commit $COMMIT_HASH."

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

LogStep "Checking out the commit to release."
git checkout $COMMIT_HASH

LogStep "Running code linter."
golangci-lint run

LogStep "Running tests."
go test -v ./...

make clean

LogStep "Creating archives for the GitHub release."
make VERSION=$RELEASE_VERSION GIT_COMMIT_HASH=$COMMIT_HASH build-archives

LogStep "Building and pushing the image."
make GIT_COMMIT_HASH=$COMMIT_HASH DOCKER_HUB_REPO=$DOCKER_HUB_REPO VERSION=$RELEASE_VERSION build-and-push-image

if IsProductionRelease
then
  GITHUB_TAG="v${RELEASE_VERSION}"
  LogStep "Pushing tag: $GITHUB_TAG to GitHub."
  git tag $GITHUB_TAG
  git push origin $GITHUB_TAG
fi

LogStep "Checking out the original commit."
git checkout $CURRENT_BRANCH

echo $RELEASE_VERSION
LogStep "The image has been released on Docker Hub and archives have been generated for a GitHub release in the build/ directory.

To complete the release, create a GitHub release for the commit $COMMIT_HASH with the version v$RELEASE_VERSION as the tag (following the format 'v0.1.0'). Upload the archives created by the release script and write release notes.
"
