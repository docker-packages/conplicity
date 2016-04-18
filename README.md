Conplicity
==========

[![Docker Pulls](https://img.shields.io/docker/pulls/camptocamp/conplicity.svg)](https://hub.docker.com/r/camptocamp/conplicity/)
[![Build Status](https://img.shields.io/travis/camptocamp/conplicity/master.svg)](https://travis-ci.org/camptocamp/conplicity)
[![By Camptocamp](https://img.shields.io/badge/by-camptocamp-fb7047.svg)](http://www.camptocamp.com)


conplicity lets you backup all your named docker volumes using duplicity.


## Examples

### Backup all named volumes to S3

```shell
$ DUPLICITY_TARGET_URL=s3://s3-eu-west-1.amazonaws.com/<my_bucket>/<my_dir> \
  AWS_ACCESS_KEY_ID=<my_key_id> \
  AWS_SECRET_ACCESS_KEY=<my_secret_key> \
    conplicity
```


### Using docker

```shell
$ docker run -v /var/run/docker.sock:/var/run/docker.sock:ro  --rm -ti \
   -e DUPLICITY_TARGET_URL=s3://s3-eu-west-1.amazonaws.com/<my_bucket>/<my_dir> \
   -e AWS_ACCESS_KEY_ID=<my_key_id> \
   -e AWS_SECRET_ACCESS_KEY=<my_secret_key> \
     camptocamp/conplicity
```

### Using the jobber-based image

A docker image based on [blacklabelops/jobber](https://hub.docker.com/r/blacklabelops/jobber/)
is provided to ease recurrent backups:

```shell
$ docker run -v /var/run/docker.sock:/var/run/docker.sock:ro  --rm -ti \
   -e DUPLICITY_TARGET_URL=s3://s3-eu-west-1.amazonaws.com/<my_bucket>/<my_dir> \
   -e AWS_ACCESS_KEY_ID=<my_key_id> \
   -e AWS_SECRET_ACCESS_KEY=<my_secret_key> \
   -e JOB_NAME1=conplicity \
   -e JOB_COMMAND1=/usr/bin/conplicity \
   -e JOB_TIME1="0 0 3" \
     camptocamp/conplicity:jobber
```


## Environment variables

### DUPLICITY_DOCKER_IMAGE

The image to use to launch duplicity. Default is `camptocamp/duplicity:latest`.

### DUPLICITY_TARGET_URL

Target URL passed to duplicity.
The hostname and the name of the volume to backup
are added to the path as directory levels.

### S3 credentials

- AWS_ACCESS_KEY_ID
- AWS_SECRET_ACCESS_KEY

### Swift credentials

- SWIFT_USERNAME
- SWIFT_PASSWORD
- SWIFT_AUTHURL
- SWIFT_TENANTNAME
- SWIFT_REGIONNAME

### Backup parameter defaults

- FULL_IF_OLDER_THAN, defaults to 15D

## Controlling backup parameters

The parameters used to backup each volume can be fine-tuned using volume labels (requires Docker 1.11.0 or greater):

- `io.conplicity.ignore=true` ignores the volume
- `io.conplicity.full_if_older_than=<value>` sets the time period after which a full backup is performed. Defaults to the `FULL_IF_OLDER_THAN` environment variable value
