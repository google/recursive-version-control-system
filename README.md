# Recursive Version Control System

This repository contains an *EXPERIMENTAL* version control system.

The aim of this experiment is to explore an alternative object model for
distributed version control systems. That model is designed to be as simple
as possible and has fewer concepts than existing DVCS's like git and Mercurial.

## Disclaimer

This is not an officially supported Google product.

## Overview

The recursive version control system (rvcs) keeps track the version history
of individual files and directories. For a directory, this history includes
the histories of all the files in that directory.

To share a file with others, you publish it by signing its history.

Published files are automatically copied to and from a set of mirrors that
you configure.

The recursive nature of the history tracking means that you can use the same
tool for tracking the history of a single file, an entire directory, or even
your entire file system.

## Status

This is *experimental* and very much a work-in-progress.

The only functionality implemented so far is the `snapshot` command.

## Usage

Snapshot the current contents of a file:

```shell
rvcs snapshot <PATH>
```

Publish the most recent snapshot of a file by signing it:

**TODO: This is planned but not yet implemented!**

```shell
rvcs publish <IDENTITY> <PATH>
```

Merge in changes from the most recent snapshot signed by someone:

**TODO: This is planned but not yet implemented!**

```shell
rvcs merge <IDENTITY> <PATH>
```

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

## Signing Snapshots

**TODO: This is planned but not yet implemented!**

You share snapshots with others by "publishing" them. This consists of signing
the snapshot by generating a signature for it tied to some identity you
control.

The rvcs tool does not mandate a specific format or type for signatures.
Instead, it allows you to configure external tools used for generating and
validating signatures.

That, in turn, is the primary extension mechanism for rvcs, as signature
types can be defined to hold any data you want.

## Mirrors

**TODO: This is planned but not yet implemented!**

The rvcs tool also does not mandate a specific mechanism for copying snapshots
between different machines, or among different users.

Instead, you configure a set of URLs as "mirrors".

When you sign a snapshot to publish it, that snapshot is automatically pushed
to these mirrors, and when you try to lookup a signed snapshot the tool
automatically reads any updated values from the mirrors.

The actual communication with each mirror is performed by an external tool
chosen based on the URL of the mirror.
