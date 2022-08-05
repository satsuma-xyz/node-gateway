# node-gateway
Blockchain node gateway.

# Development
## Commit message linting
CI enforces that commit messages meet the [conventional commit format](https://conventionalcommits.org). For convenience of local development, there's a commit-msg Git hook that verifies if the commit message is valid. To use it, create a symlink:
```
ln -s $PWD/scripts/git_hooks/* .git/hooks
```
