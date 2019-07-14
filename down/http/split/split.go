package split

var (
	minBurset int64
)

func Init(burset int64)  {
	minBurset = burset
}

func CaculateBurst(size int64) (count int, chunkSize int64) {
	countTemp := size / minBurset
	if countTemp == 0 {
		countTemp = 1
	}
	chunkSize = size / countTemp
	return int(countTemp), chunkSize
}