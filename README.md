# nuvi
nuvi assignment

## Installation
```shell
go get -u github.com/joeybloggs/nuvi
```

## DB 
This connects to a Redis database running at localhost:6379 with no password and database 0.

## Requirements
- [x] Download all zip files from given URL
- [x] Extract XML files from .zip + publish to Redis list
- [x] Make application idempotent, runnable multiple times without duplicating data

## Assumptions
Without know the context what kind of machine this will eventually run on and how the Redis list will be consumed the following assumptions were made.

- Raw XML was stored in Redis instead of compressed to save space.
- The XML added to the list in descending order, by .zip creation time, for direct read to display use.

## Additional Features
Some additional features implemented

- Tunable parallel downloading of .zip files but maintains FIFO to maintain list order.

## TO DO

- Make Redis DB information configurage via flags + ENV variables.
- Create generic parallel download library for use in multiple projects.