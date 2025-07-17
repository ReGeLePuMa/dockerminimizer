# Dockerminimizer

## Description

A CLI tool to **automatically minimize Dockerfiles** by analyzing the dependencies of containerized applications.

## Features

- **Static Analysis**: Identifies linked libraries using `ldd`.
- **Dynamic Analysis**: Uses `strace` to monitor `execve` and  `open` calls to determine runtime dependencies.
- **Binary Search**: If dynamic analysis fails, it tries to minimize the container using a binary search approach.

## Benchmarks

The tool has been tested on various applications, yielding the following results:

![image](https://i.imgur.com/Cr4HGlA.png)

## Installation

To install **dockerminimizer** locally, you can use the following command:

```bash
git clone https://github.com/ReGeLePuMa/dockerminimizer.git
cd dockerminimizer
./install.sh
```

Alternatively, you can use the following [container image](https://hub.docker.com/r/regelepuma/dockerminimizer):

```bash
docker run \
    --rm \
    --privileged \
    -v /root/.dockerminimizer:/root/.dockerminimizer \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v $(pwd):/app \
    regelepuma/dockerminimizer
```

## Usage

```bash
dockerminimizer --help

A tool to minimize Dockerfiles by determining the dependencies of the containerized application

Usage:
  dockerminimizer [flags]

Flags:
      --binary_search        Continue with binary search if dynamic analysis fails (default true)
      --debug                Enable debug mode
  -f, --file string          Path to the Dockerfile (default "./Dockerfile")
  -h, --help                 help for dockerminimizer
  -i, --image string         Name of the Docker image
      --max_limit int        Number of binary search steps (default 10)
      --strace_path string   Path to the statically linked strace binary (default "/usr/local/bin/strace")
      --timeout int          How long the container should run before being declared healthy (default 30)
```

## Example

For [this NodeJS](https://github.com/ReGeLePuMa/testing/blob/be1d8328686cb7b5f554b0009eba7967815e6cb9/url_shortener/app.js) app, with the following Dockerfile:

```dockerfile
FROM node:20.9.0

WORKDIR /app

COPY app.js /app/app.js
COPY package.json /app/package.json
COPY package-lock.json /app/package-lock.json

RUN npm install
EXPOSE 3000
CMD ["node", "app.js"]
```

Running the command:

```bash
dockerminimizer -f Dockerfile
```

will produce a new Dockerfile in the current directory named `Dockerfile.minimal`:

```dockerfile
FROM node:20.9.0

WORKDIR /app

COPY app.js /app/app.js
COPY package.json /app/package.json
COPY package-lock.json /app/package-lock.json

RUN npm install
EXPOSE 3000
CMD ["node", "app.js"]


FROM scratch

ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
WORKDIR /app
EXPOSE 3000/tcp
CMD ["node", "app.js"]

COPY --from=builder ["/app/node_modules/morgan/node_modules/debug/package.json", "/app/node_modules/morgan/node_modules/debug/"]
COPY --from=builder ["/app/node_modules/body-parser/package.json", "/app/node_modules/body-parser/index.js", "/app/node_modules/body-parser/"]
COPY --from=builder ["/app/node_modules/ipaddr.js/lib/ipaddr.js", "/app/node_modules/ipaddr.js/lib/"]
COPY --from=builder ["/app/node_modules/qs/lib/index.js", "/app/node_modules/qs/lib/stringify.js", "/app/node_modules/qs/lib/utils.js", "/app/node_modules/qs/lib/formats.js", "/app/node_modules/qs/lib/parse.js", "/app/node_modules/qs/lib/"]
.
.
.
```
### Result

![image](https://i.imgur.com/3Snxs4z.png)
