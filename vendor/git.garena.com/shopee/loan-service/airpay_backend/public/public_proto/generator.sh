#!/bin/bash

protoc --go_out=plugins=grpc,paths=source_relative:./merchant  txn_merchant_info.proto -I./merchant
protoc --go_out=plugins=grpc,paths=source_relative:./device  txn_device_info.proto -I./device
protoc --go_out=plugins=grpc,paths=source_relative:./ common.proto


