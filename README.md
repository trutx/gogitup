# gogitup

## Overview

This tool updates all git repositories configured in a yaml file in a fancy, multithreaded way. Also suports fork updating.

Under the hood, the used commands are:

* `git pull` for regular cloned repositories
* `git fetch` + `git rebase <upstreamName>/master` + `git push` for forks

## Configuration

Sample configuration file:

```
# threads defaults to 5
# colors defaults to true
# followSymlinks defaults to false
# isFork defaults to false
# upstreamName defaults to 'upstream'
# Configuration example:

threads: 10
colors: false
followSymlinks: true
ignore:
  - .DS_Store
  - /repos/ignoreme
repos:
  - path: ~/repos/singlerepo
  - path: ~/mycode/singlefork
    isFork: true
  - path: /opt/anotherfork
    isFork: true
    upstreamName: mysource
  - path: ~/repos/myorg/*
  - path: ~/repos/myforks/*
    isFork: true
```

The default configuration file is `~/.gogitup.yaml`, but you can specify any other path using the `-c` flag:

`$ gogitup`

`$ gogitup -c /etc/myconfig.yaml`

## Installation

To install, just `go get github.com/trutx/go/gogitup`
