set -x
cd ./Application
go build app.go

cd ../Controle
go build ctl.go

cd ..

rm sauvegarde.json

rm -f /tmp/in_A1
rm -f /tmp/in_A2
rm -f /tmp/in_A3
rm -f /tmp/in_C1
rm -f /tmp/in_C2
rm -f /tmp/in_C3
rm -f /tmp/out_A1
rm -f /tmp/out_A2
rm -f /tmp/out_A3
rm -f /tmp/out_C1
rm -f /tmp/out_C2
rm -f /tmp/out_C3

mkfifo /tmp/in_A1 /tmp/out_A1
mkfifo /tmp/in_C1 /tmp/out_C1

mkfifo /tmp/in_A2 /tmp/out_A2
mkfifo /tmp/in_C2 /tmp/out_C2

mkfifo /tmp/in_A3 /tmp/out_A3
mkfifo /tmp/in_C3 /tmp/out_C3

./Application/app -s 1 -nb 50 -port 4411 < /tmp/in_A1 > /tmp/out_A1  2>/tmp/err_A1 &
./Controle/ctl -s 1 -n 0 < /tmp/in_C1 > /tmp/out_C1 2>/tmp/err_C1 &

./Application/app -s 2 -nb 50 -port 4421  < /tmp/in_A2 > /tmp/out_A2 2>/tmp/err_A2 &
./Controle/ctl -s 2 -n 0 < /tmp/in_C2 > /tmp/out_C2 2>/tmp/err_C2 &

./Application/app -s 3 -nb 50 -port 4431 < /tmp/in_A3 > /tmp/out_A3 2>/tmp/err_A3 &
./Controle/ctl -s 3 -n 0 < /tmp/in_C3 > /tmp/out_C3 2>/tmp/err_C3 &
 
cat /tmp/out_A1 >> /tmp/in_C1 &
cat /tmp/out_C1 | tee /tmp/in_A1 >> /tmp/in_C2 &

cat /tmp/out_A2 >> /tmp/in_C2 &
cat /tmp/out_C2 | tee /tmp/in_A2 >> /tmp/in_C3 &

cat /tmp/out_A3 >> /tmp/in_C3 &
cat /tmp/out_C3 | tee /tmp/in_A3 >> /tmp/in_C1 &

tail -f /tmp/err_C1 /tmp/err_C2 /tmp/err_C3 &
tail -f /tmp/err_A1 /tmp/err_A2 /tmp/err_A3 &
