# Recursive Version Control System

This repository contains an *EXPERIMENTAL* version control system.

The aim of this experiment is to explore an alternative object model for
distributed version control systems. That model is designed to be as simple
as possible and has fewer concepts than existing DVCS's like git and Mercurial.

## Disclaimer

This is not an officially supported Google product.

## Overview

The recursive version control system (rvcs) tracks the version history of
individual files and directories. For a directory, this history includes the
histories of all the files in that directory.

That hierarchical structure means you can share your files at any level
and can share different files/directories with different audiences.

To share a file with others, you publish it by signing its history.

Published files are automatically copied to and from a set of mirrors that
you configure.

The recursive nature of the history tracking means that you can use the same
tool for tracking the history of a single file, an entire directory, or even
your entire file system.

## Usage

Snapshot the current contents of a file:

```shell
rvcs snapshot <PATH>
```

Publish the most recent snapshot of a file by signing it:

```shell
rvcs publish <PATH> <IDENTITY>
```

Merge in changes from the most recent snapshot signed by someone:

```shell
rvcs merge <IDENTITY> <PATH>
```

## Getting Started

### Installation

If you have the [Go tools installed](https://golang.org/doc/install), you can
install the `rvcs` tool by running the following command:

    go install github.com/google/recursive-version-control-system/cmd/...@latest

Then, make sure that `${GOPATH}/bin` is in your PATH.

Optionally, you can also copy the files from the `extensions` directory into some directory in your PATH to use them for publishing snapshots.

### Example Setup And Usage

The `extensions` directory includes helpers for using SSH public keys as
identities and local filesystem paths as mirrors. So, if you have them
installed then you can set up an example identity and mirror using the
following commands:

```shell
ssh-keygen -t ed25519 -f ~/.ssh/rvcs_example -C "Example identity for RVCS"
export RVCS_EXAMPLE_IDENTITY="ssh::$(cat ~/.ssh/rvcs_example.pub | cut -d ' ' -f 2)"
mkdir -p ${HOME}/rvcs-example/local-filesystem-mirror
export RVCS_EXAMPLE_MIRROR="file://${HOME}/rvcs-example/local-filesystem-mirror"
rvcs add-mirror ${RVCS_EXAMPLE_IDENTITY} ${RVCS_EXAMPLE_MIRROR}
```

Then you can publish an example directory to that identity with the following:

```shell
mkdir -p ${HOME}/rvcs-example/dir-to-publish
echo "Hello, World\!" > ${HOME}/rvcs-example/dir-to-publish/hello.txt
rvcs snapshot ${HOME}/rvcs-example/dir-to-publish
rvcs publish ${HOME}/rvcs-example/dir-to-publish ${RVCS_EXAMPLE_IDENTITY}
```

If you share the local mirror (the directory you created at
`${HOME}/rvcs-example/local-filesystem-mirror`) with another user, they
can then retrieve your published snapshot with:

```shell
rvcs add-mirror --read-only ${RVCS_EXAMPLE_IDENTITY} ${RVCS_EXAMPLE_MIRROR}
rvcs merge ${RVCS_EXAMPLE_IDENTITY} ~/rvcs-example-merge
```

After you are done with the example, you can clean up by removing the mirror:

```shell
rvcs remove-mirror ${RVCS_EXAMPLE_IDENTITY} ${RVCS_EXAMPLE_MIRROR}
```

## Status

This is *experimental* and very much a work-in-progress.

In particular, the tool is still subject to change and there is no guarantee
of backwards compatibility.

The `snapshot` command is fully implemented, and no changes are currently
planned for it, but that is subject to change.

The `publish` and `merge` commands are both implemented, but rely on external
helper commands in order to actually use them.

The `merge` command defaults to using the widely-available `diff3` command if
no helper is provided. For the `publish` command, there are proof of concept
helpers provided in the `extensions` directory.

## Model

The core concept in rvcs is a `snapshot`. A snapshot describes a point-in-time
in the history of a file, where the file might be a regular file or a directory
containing other files.

Each snapshot contains a fixed set of metadata about the file (such as whether
or not it is a directory), a link to the contents of the file at that point,
and links to any other snapshots that came immediately before it.

These links in a snapshot are of the form `<hashfunction>:<hexadecimalstring>`,
where `<hashfunction>` is the name of a specific
[function](https://en.wikipedia.org/wiki/Hash_function) used to generate
a hash, and `<hexadecimalstring>` is the generated hash of the thing being
referenced. Currently, the only supported hash function is
[sha256](https://en.wikipedia.org/wiki/SHA-2).

When the snapshot is for a directory, the contents are a plain text file
listing the names of each file contained in that directory, and that file's
corresponding snapshot.

## Publishing Snapshots

You share snapshots with others by "publishing" them. This consists of signing
the snapshot by generating a signature for it tied to some identity you
control.

The rvcs tool does not mandate a specific format or type for signatures.
Instead, it allows you to configure external tools used for generating and
validating signatures.

That, in turn, is the primary extension mechanism for rvcs, as signature
types can be defined to hold any data you want.

### Sign and Verify Helpers

Identities are of the form `<namespace>::<contents>`. In order to be able
to publish a snapshot with a given identity, you must have "sign" and "verify"
helpers located somewhere in your local system path.

These helpers will always be named of the form `rvcs-sign-<namespace>` and
`rvcs-verify-<namespace>`, where `<namespace>` is the prefix of the identity
that comes before the first pair of colons.

So, for example, to publish a snapshot with the identity `example::user`,
you must have two programs in your system path named `rvcs-sign-example` and
`rvcs-verify-example`.

The sign helper takes four arguments; the full contents of the
identity (e.g. `example::user` for the example above), the hash of the
snapshot to sign, the hash of the previous signature created for that
identity (or the empty string if there is none), and a file to which it
writes its output.

If it is successful, then it writes to the output file the hash of the
snapshot of the generated signature and exits with a status code of `0`.

The verify helper does the reverse of that. It takes three arguments; the
identity, the hash of the generated signature, and a file to write output.
It then verifies that this signature is valid for the specified identity.

If it is, then the verify helper outputs the hash of the signed snapshot
and exits with a status code of `0`.

There are example sign and verify helpers in the `extensions` directory that
demonstrate how to sign and verify signatures using SSH keys.

## Mirrors

The rvcs tool also does not mandate a specific mechanism for copying snapshots
between different machines, or among different users.

Instead, you configure a set of URLs as "mirrors".

When you sign a snapshot to publish it, that snapshot is automatically pushed
to these mirrors, and when you try to lookup a signed snapshot the tool
automatically reads any updated values from the mirrors.

The actual communication with each mirror is performed by an external tool
chosen based on the URL of the mirror.

### Push and Pull Helpers

Similarly to the sign and verify helpers, the rvcs tool relies on push and
pull helpers to push snapshots to and pull them from mirrors.

The helper tools are named of the form `rvcs-push-<scheme>` and
`rvcs-pull-<scheme>`, where `<scheme>` is the scheme portion of the mirror's
URL.

So, for example, if a mirror has the URL `file:///some/local/path`, then
rvcs will try to invoke a tool named `rvcs-push-file` to push to that mirror
and one named `rvcs-pull-file` to pull from it.

The pull helper tool takes the full URL of the mirror (including the scheme),
the fully specified identity (including the namespace), the hash of the most
recently-known signature for that identity, and a file for it to write output.

When successfull it outputs the hash of the latest signature for that
identity that it pulled from the mirror and exits with a status code of `0`.

The push helper takes the full URL of the mirror, the fully specified
identity, the hash of the latest, updated signature for that identity, and
a file for it to write output.

If it successfully pushes that update to the mirror then it outputs the
hash of the signature that was pushed and exits with a status code of `0`.

There are example push and pull helpers in the `extensions` directory that
demonstrate how to use a local file path as a mirror.

## Merging

The `rvcs` provides a `merge` subcommand to automatically merge different
snapshots together and then checkout the result into some local file path.

The `merge` command takes two arguments, the "left hand side" of the merge
and the "right hand side".

The left hand side can be any type of reference to a snapshot, while the
right hand side must be a local file system path.

If the merge is successful, then the file system contents of the path
provided for the right hand side are updated to match the merged snapshot.

### Merge Helpers

The `rvcs` tool will do its best to automatically merge changes to directories,
but if there are conflicting changes to individual files it relies on an
external helper command to try to automatically merge them.

By default, it uses the `diff3` command to perform this merge. That can be
changed by specifying the name of the command to use in the
`RVCS_MERGE_HELPER_COMMAND` environment variable.

If the supplied merge helper requires extra arguments, then they can be
specified in a JSON-encoded list using the `RVCS_MERGE_HELPER_ARGS` environment
variable.

The `rvcs` tool invokes the specified merge helper with the specified args,
followed by the paths to three files:

1. A file with the same contents and history as the left-hand side of the
   merge.
2. A file with the same contents and history as the most recent common
   ancestor of both sides of the merge.
3. A file with the same contents and history as the right-hand side of the
   merge.

If the merge helper exits with a status of `0`, then its standard output is
taken as the contents of the successfully-merged file.

Otherwise, the automatic merge fails and you have to manually merge the
changes.

### Manual Merges

The `rvcs` tool enables version control for files located anywhere on your
computer. This fact enables a workflow that we refer to as manual merging.

This is the fallback that you can use to merge conflicting changes in the
event that `rvcs` is not able to automatically merge them.

To perform a manual merge you create a temporary directory where you will
merge the conflicting versions.

You then check out the right-hand-side of your conflicting merge into this
temporary directory:

```shell
rvcs merge ${RIGHT_HAND_SIDE} ${TEMPORARY_DIRECTORY}/${FILENAME}
```

Optionally, you can also check out the left-hand-side into this same
directory if you want to see the changes side-by-side:

```shell
rvcs merge ${LEFT_HAND_SIDE} ${TEMPORARY_DIRECTORY}/other_${FILENAME}
```

Next, you manually edit the contents of the checked out temporary file
to look the way you want.

After that you can create a manual merge of the two sides using the
`rvcs snapshot` command with the `--additional-parents` flag:

```shell
rvcs snapshot --additional-parents=${LEFT_HAND_SIDE} ${TEMPORARY_DIRECTORY}/${FILENAME}
```

This new snapshot will have both sides of the merge as its parents, so
you can then merge it into your destination path:

```shell
rvcs merge ${TEMPORARY_DIRECTORY}/${FILENAME} ${RIGHT_HAND_SIDE}
```

Finally, you can clean up by removing the temporary directory.
