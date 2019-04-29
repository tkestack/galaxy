# Contributing to Galaxy

Welcome to report Issues or send pull requests. It's recommended to read the following Contributing Guide first before contributing.

# Issues

## Search Known Issues First

Please search the existing issues to see if any similar issue or feature request has already been filed. You should make sure your issue isn't redundant.

## Reporting New Issues

If you open an issue, the more information the better. Such as detailed description, screenshot or video of your problem, logcat or code blocks for your crash.

# Pull Requests

We strongly welcome your pull request to make Galaxy better.

## Branch Management

There are two main branches here:

1. master branch

The developing branch. We welcome bugfix, features, typo and whatever on this branch.

2. branch-* branch

The releasing branch. It is our stable release branch. You are recommended to submit bugfix only to these branches.

## Make Pull Requests

The code team will monitor all pull request. Before submitting a pull request, please make sure the followings are done:

1. Fork the repo and create your branch from master.
1. Update code or documentation if you have changed APIs.
1. Check your code lints and checkstyles.
1. Test your code.
1. Submit your pull request to master branch.

# Code Style Guide

We use [gometalinter](https://github.com/alecthomas/gometalinter) to check code styles. Make sure `gometalinter cni/... cmd/... pkg/... tools/...` passes.

# Generate API docs

Galaxy provides swagger 1.2 docs. Add `--swagger` command line args to galaxy-ipam and restart it, check `http://${galaxy-ipam-ip}:9041/apidocs.json/v1`
API documents of this project is converted by [api-spec-converter](https://github.com/LucyBot-Inc/api-spec-converter) and [swagger-markdown](https://github.com/syroegkin/swagger-markdown).

```
# convert swagger docs to 2.0 format
api-spec-converter --from=swagger_1 --to=swagger_2 --syntax=yaml --order=alpha http://${galaxy-ipam-ip}:9041/apidocs.json/v1 > swagger.json

# convert swagger 2.0 json doc to markdown format. This will output a swagger.md
swagger-markdown -i swagger.json
```

# License

By contributing to Galaxy, you agree that your contributions will be licensed under its [Apache License 2.0](../LICENSE)
