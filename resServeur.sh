set -x

cd ./Web
go build serveur.go

cd ..

#Lancement des serveurs en dernier car ils se connectent aux websockets des apps

./Web/serveur -appHost localhost -appPort 4411 -port 4410 &
./Web/serveur -appHost localhost -appPort 4421 -port 4420 &
./Web/serveur -appHost localhost -appPort 4431 -port 4430 &

