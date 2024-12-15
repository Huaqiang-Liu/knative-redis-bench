package shared

import (
	"encoding/csv"
	"math/rand"
	"os"
	"strconv"
)

// 全局变量，免得老读文件
const SECOND_OF_A_DAY = 3600 * 24

var chainLenCDF map[int]float64
var invokesCDF map[int]float64
var CVsCDF map[float64]float64

func init() {
	chainLenCDF = make(map[int]float64)
	invokesCDF = make(map[int]float64)
	CVsCDF = make(map[float64]float64)

	chainCDFFile, _ := os.Open("./CDFs/chainlenCDF.csv")
	reader := csv.NewReader(chainCDFFile)
	_, _ = reader.Read() // 跳过第一行
	for {
		record, _ := reader.Read()
		if record == nil {
			break
		}
		lengthStr := record[0]
		fxStr := record[1]

		length, _ := strconv.Atoi(lengthStr)
		fx, _ := strconv.ParseFloat(fxStr, 64)

		chainLenCDF[length] = fx
	}
	chainCDFFile.Close()

	invokesCDFFile, _ := os.Open("./CDFs/invokesCDF.csv")
	reader = csv.NewReader(invokesCDFFile)
	for {
		record, _ := reader.Read()
		if record == nil {
			break
		}
		invokeTimeStr := record[0]
		fxStr := record[1]

		invokeTime, _ := strconv.Atoi(invokeTimeStr)
		fx, _ := strconv.ParseFloat(fxStr, 64)

		invokesCDF[invokeTime] = fx
	}
	invokesCDFFile.Close()

	CVsCDFFile, _ := os.Open("./CDFs/CVs.csv")
	reader = csv.NewReader(CVsCDFFile)
	for {
		record, _ := reader.Read()
		if record == nil {
			break
		}
		cvStr := record[0]
		fxStr := record[1]

		cv, _ := strconv.ParseFloat(cvStr, 64)
		fx, _ := strconv.ParseFloat(fxStr, 64)

		CVsCDF[cv] = fx
	}
	CVsCDFFile.Close()
}

// 每个sequence只调用一次
func GetSeqLen() int {
	rnd := rand.Float64()
	for length, fx := range chainLenCDF {
		if rnd <= fx {
			return length
		}
	}
	// 如果随机数大于所有的累积分布值(其实这是不可能的，因为累积分布值最大是1)，返回-1
	return -1
}

func getRandAvgIAT() float64 {
	rnd := rand.Float64()
	for avgIAT, fx := range invokesCDF {
		if rnd <= fx {
			return float64(SECOND_OF_A_DAY) / float64(avgIAT)
		}
	}
	return 0
}

func getRandCV() float64 {
	rnd := rand.Float64()
	for cv, fx := range CVsCDF {
		if rnd <= fx {
			return cv
		}
	}
	return 0
}

func GetRandomIAT() float64 { // 每个任务到达activator时都要调用它
	avgIAT := getRandAvgIAT()
	cv := getRandCV()
	stdDev := avgIAT * cv
	iat := rand.NormFloat64()*stdDev + avgIAT
	for iat <= 0 {
		iat = rand.NormFloat64()*stdDev + avgIAT
	}
	return iat
}
