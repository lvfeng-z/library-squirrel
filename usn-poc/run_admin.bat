@echo off
cd /d E:\code\lvfeng\library-squirrel
set CGO_ENABLED=0
go run ./usn-poc/ > usn-poc\admin_out.txt 2>&1
