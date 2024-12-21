# Release Guide

These steps describe how to cut a release of the catalogd repo.

## Table of Contents:

- [Major and minor releases](#major-and-minor-releases)

## Major and Minor Releases

Before starting, ensure the milestone is cleaned up. All issues that need to
get into the release should be closed and any issue that won't make the release
should be pushed to the next milestone.

These instructions use `v1.MINOR.PATCH` as the example release. Please ensure to replace
the version with the correct release being cut. It is also assumed that the upstream
operator-framework/catalogd repository is the `upstream` remote on your machine.

### Procedure

1. Create a release branch by running the following, assuming the upstream
operator-framework/catalogd repository is the `upstream` remote on your machine:

   - ```sh
     git checkout main
     git fetch upstream
     git pull upstream main
     git checkout -b release-v1.Y
     git push upstream release-v1.Y
     ```

2. Tag the release:

   - ```sh
     git tag -am "catalogd v1.Y.0" v1.Y.0
     git push upstream v1.Y.0
     ```

3. Check the status of the [release GitHub Action](https://github.com/operator-framework/catalogd/actions/workflows/release.yaml).
Once it is complete, the new release should appear on the [release page](https://github.com/operator-framework/catalogd/releases).

### Procedure for Patch Releases

1. Create Pull requests against the release branch with cherry-picks of the commits that need to be included in the patch release.

   - ```sh
     git checkout -b release-v1.Y
     git checkout -b my-cherry-pick-branch
     git cherry-pick -x <commit-hash>
     git push origin +my-cherry-pick-branch
     ```
     
2. Once all required PRs cherry-picks are merged and we are prepared to cut the patch release, create the PATCH release from the branch:

   - ```sh
     git checkout main
     git fetch upstream
     git checkout release-v1.Y
     git tag -am "catalogd v1.Y.PATCH" v1.Y.PATCH
     git push upstream v1.Y.PATCH
     ```

3. Check the status of the [release GitHub Action](https://github.com/operator-framework/catalogd/actions/workflows/release.yaml).
   Once it is complete, the new release should appear on the [release page](https://github.com/operator-framework/catalogd/releases).

## Backporting Policy

Mainly critical issue fixes are backported to the most recent minor release.
Special backport requests can be discussed during the weekly Community meeting or via Slack channel; 
this does not guarantee an exceptional backport will be created. 

Occasionally non-critical issue fixes will be backported, either at an approver’s discretion or by request as noted above. 
For information on contacting maintainers and attending meetings, check the community repository.

### Process

1. Create a PR with the fix cherry-picked against to the release branch.
2. Ask for a review from the maintainers.
3. Once approved, merge the PR and perform the Patch Release steps above.

