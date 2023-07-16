
set -x

for pid in $(ps -ef | grep Web/serveur | grep -v grep | awk '{print $2}')
do
	kill $pid
done
ps -f
