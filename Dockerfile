# syntax=docker/dockerfile:1

FROM golang:1.17.1-alpine
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY . ./
ARG HOST=${HOST}
ARG PORT=${PORT}
ARG TELEGRAM_TOKEN=${TELEGRAM_TOKEN}
RUN go build -o /pacer
CMD [ "/" ]