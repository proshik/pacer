# syntax=docker/dockerfile:1

FROM golang:1.17.1-alpine
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY . ./
ARG HOST="Default_Value"
ARG PORT="Default_Value"
ARG TELEGRAM_TOKEN="Default_Value"
RUN go build -o /pacer
CMD [ "/" ]