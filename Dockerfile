# syntax=docker/dockerfile:1

FROM golang:1.17.1-alpine
ARG HOST=${HOST}
ARG PORT=${PORT}
ARG TELEGRAM_TOKEN=${TELEGRAM_TOKEN}
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY . ./
RUN go build -o /pacer
CMD [ "/" ]