# nuvi
nuvi assignment

## Requirements
- [x] Download all zip files from given URL
- [x] Extract XML files from .zip + publish to Redis list
- [x] Make application idempotent, runnable multiple times without duplicating data

## Assumptions
Without know the context what kind of machine this will eventually run on and how the Redis list will be consumed the following assumptions were made.

- Raw XML was stored in Redis instead of compressed to save space.
- Application built for pure speed. eg. all operations done in memory avoiding hard disk bottlenecks, but consumes more memory.
- The XML added to the list in descending order, by .zip creation time, for direct read to display use.

## Additional Features
Some additional features implemented

- Tunable parallel downloading of .zip files but maintains FIFO to maintain list order.
