# node-gateway
Blockchain node gateway.

# Development

## Code linting
`golangci-lint` can be run in your local editor for quick feedback. For VSCode:
1. Install the [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.Go)
2. [Install `golangci-lint`](https://golangci-lint.run/usage/install/). 
3. Add the following to your `settings.json` file:
```
{
  "go.lintTool":"golangci-lint",
  "go.lintFlags": [
    "--fast"
  ]
}
```

## Commit message linting
CI enforces that commit messages meet the [conventional commit format](https://conventionalcommits.org). For convenience of local development, there's a commit-msg Git hook that verifies if the commit message is valid. To use it, create a symlink:
```
ln -s $PWD/scripts/git_hooks/* .git/hooks
```
