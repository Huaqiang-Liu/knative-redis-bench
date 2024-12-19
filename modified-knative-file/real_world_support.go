package shared

import (
	"encoding/csv"
	"math/rand"
	"os"
	"strconv"
)

// 全局变量，免得老读文件
const SECOND_OF_A_DAY = 3600 * 24

type chainLen struct {
	key   int
	value float64
}
type invokes struct {
	key   int
	value float64
}
type CVs struct {
	key   float64
	value float64
}
type execTime struct {
	key   int
	value float64
}

var chainLenCDF []chainLen
var invokesCDF []invokes
var CVsCDF []CVs
var execTimeCDF []execTime

func init() {
	chainLenCDF = make([]chainLen, 0)
	invokesCDF = make([]invokes, 0)
	CVsCDF = make([]CVs, 0)
	execTimeCDF = make([]execTime, 0)

	chainCDFFile, _ := os.Open("/app/CDFs/chainlenCDF.csv")
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

		chainLenCDF = append(chainLenCDF, chainLen{length, fx})
	}
	chainCDFFile.Close()

	invokesCDFFile, _ := os.Open("/app/CDFs/invokesCDF.csv")
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

		invokesCDF = append(invokesCDF, invokes{invokeTime, fx})
	}
	invokesCDFFile.Close()

	CVsCDFFile, _ := os.Open("/app/CDFs/CVs.csv")
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

		CVsCDF = append(CVsCDF, CVs{cv, fx})
	}
	CVsCDFFile.Close()

	execTimeCDFFile, _ := os.Open("/app/CDFs/execTimeCDF.csv")
	reader = csv.NewReader(execTimeCDFFile)
	for {
		record, _ := reader.Read()
		if record == nil {
			break
		}
		exectimeStr := record[0]
		fxStr := record[1]

		exectime, _ := strconv.Atoi(exectimeStr)
		fx, _ := strconv.ParseFloat(fxStr, 64)

		execTimeCDF = append(execTimeCDF, execTime{exectime, fx})
	}
}

// 每个sequence只调用一次
func GetSeqLen() int {
	rnd := rand.Float64()
	for _, cl := range chainLenCDF {
		if rnd <= cl.value {
			return cl.key
		}
	}
	// 如果随机数大于所有的累积分布值(其实这是不可能的，因为累积分布值最大是1)，返回-1
	return -1
}

func GetRandAvgIAT() float64 {
	rnd := rand.Float64()
	for _, iv := range invokesCDF {
		if rnd <= iv.value {
			return float64(iv.key)
		}
	}
	return -1
}

func GetRandCV() float64 {
	rnd := rand.Float64()
	for _, cv := range CVsCDF {
		if rnd <= cv.value {
			return cv.key
		}
	}
	return -1
}

func GetRandIAT(avgIAT float64, cv float64) float64 { // 每个任务到达activator时都要调用它
	stdDev := avgIAT * cv
	iat := rand.NormFloat64()*stdDev + avgIAT
	for iat <= 0 {
		iat = rand.NormFloat64()*stdDev + avgIAT
	}
	return iat
}

func GetRandExecTime() int {
	rnd := rand.Float64()
	for _, et := range execTimeCDF {
		if rnd <= et.value {
			return et.key
		}
	}
	return -1
}

// 根据执行时间返回其所属组下标
func GetGroupIndex(execTime int) int {
	for i := 0; i < 10; i++ {
		if JoblenEdge[i] > execTime {
			return i
		}
	}
	return -1
}
