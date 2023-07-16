
set -x

for pid in $(ps -ef | grep tion/app | grep -v grep | awk '{print $2}')
do
	kill $pid
done
ps -f

for pid in $(ps -ef | grep role/ctl | grep -v grep | awk '{print $2}')
do
	kill $pid
done
ps -f

for pid in $(ps -ef | grep tail | grep -v grep | awk '{print $2}')
do
	kill $pid
done
ps -f
