package mnist

import (
	"bufio"
	"io"
	"math/rand"
	"os"
	"pfi/sensorbee/sensorbee/bql"
	"pfi/sensorbee/sensorbee/core"
	"pfi/sensorbee/sensorbee/data"
	"time"
)

// DataSourceCreator is a creator for MNIST data source.
type DataSourceCreator struct{}

// mnistDataSource is a source to generate MNIST data stream.
type mnistDataSource struct {
	data          [][]float32
	target        []int32
	dataSize      int
	imageElemSize int
	batchSize     int
	randomFlag    bool
}

var (
	imagesFileNamePath = data.MustCompilePath("images_file_name")
	labelsFileNamePath = data.MustCompilePath("labels_file_name")
	dataSizePath       = data.MustCompilePath("data_size")
	imageElemSizePath  = data.MustCompilePath("image_element_size")
	batchSizePath      = data.MustCompilePath("batch_size")
	randomFlagPath     = data.MustCompilePath("random")
)

// CreateSource returns a source which generate MNIST data stream. The MNIST
// data spec is depended on "THE MNIST DATABASE of handwritten digits", see
// http://yann.lecun.com/exdb/mnist/
//
// MNIST data is ubyte data, and parse when the source is created. Returns
// an error when cannot load MNIST file or parsing error.
//
// WITH parameters:
//  images_file_name:   MNIST images nbytes file path [required]
//  labels_file_name:   MNIST labels nbytes file path [required]
//  data_size:          MNIST data size [required]
//  image_element_size: MNIST image element size (default: 784=28*28)
//  batch_size:         batch size [required]
//  random:             randomize data on/off (default: true)
func (s *DataSourceCreator) CreateSource(ctx *core.Context, ioParams *bql.IOParams,
	params data.Map) (core.Source, error) {
	ms, err := createMNISTDataSource(ctx, ioParams, params)
	if err != nil {
		return nil, err
	}

	return core.NewRewindableSource(ms), nil
}

func createMNISTDataSource(ctx *core.Context, ioParams *bql.IOParams,
	params data.Map) (core.Source, error) {

	imagesDataName := ""
	if idn, err := params.Get(imagesFileNamePath); err != nil {
		return nil, err
	} else if imagesDataName, err = data.AsString(idn); err != nil {
		return nil, err
	}

	labelsDataName := ""
	if ldn, err := params.Get(labelsFileNamePath); err != nil {
		return nil, err
	} else if labelsDataName, err = data.AsString(ldn); err != nil {
		return nil, err
	}

	dataSize := 0
	if ds, err := params.Get(dataSizePath); err != nil {
		return nil, err
	} else if dsInt, err := data.AsInt(ds); err != nil {
		return nil, err
	} else {
		dataSize = int(dsInt)
	}

	imageElemSize := 28 * 28
	if ies, err := params.Get(imageElemSizePath); err == nil {
		iesInt, err := data.AsInt(ies)
		if err != nil {
			return nil, err
		}
		imageElemSize = int(iesInt)
	}

	batchSize := 1
	if bs, err := params.Get(batchSizePath); err != nil {
		return nil, err
	} else if bsInt, err := data.AsInt(bs); err != nil {
		return nil, err
	} else {
		batchSize = int(bsInt)
	}

	randomFlag := true
	if flag, err := params.Get(randomFlagPath); err == nil {
		randomFlag, err = data.AsBool(flag)
		if err != nil {
			return nil, err
		}
	}

	target, data, err := getMNISTRawData(imagesDataName, labelsDataName, dataSize,
		imageElemSize)
	if err != nil {
		return nil, err
	}

	ms := &mnistDataSource{
		data:          data,
		target:        target,
		dataSize:      dataSize,
		imageElemSize: imageElemSize,
		batchSize:     batchSize,
		randomFlag:    randomFlag,
	}

	return ms, nil
}

const (
	imagesDataOffsetSize = 16
	labelsDataOffsetSize = 8
)

func getMNISTRawData(imagesDataName string, labelsDataName string, dataSize int,
	imageElemSize int) ([]int32, [][]float32, error) {

	imagesData := dataSource{path: imagesDataName}
	ir, ic, err := imagesData.reader()
	if err != nil {
		return []int32{}, [][]float32{}, err
	}
	defer ic.Close()

	labelsData := dataSource{path: labelsDataName}
	lr, lc, err := labelsData.reader()
	if err != nil {
		return []int32{}, [][]float32{}, err
	}
	defer lc.Close()

	data := make([][]float32, dataSize, dataSize)
	for i := range data {
		data[i] = make([]float32, imageElemSize, imageElemSize)
	}
	target := make([]int32, dataSize, dataSize)

	for i := 0; i < imagesDataOffsetSize; i++ {
		ir.ReadByte()
	}
	for i := 0; i < labelsDataOffsetSize; i++ {
		lr.ReadByte()
	}
	for i := 0; i < dataSize; i++ {
		lb, _ := lr.ReadByte()
		target[i] = int32(lb)
		for j := 0; j < imageElemSize; j++ {
			ib, _ := ir.ReadByte()
			data[i][j] = float32(ib) / 255
		}
	}

	return target, data, nil
}

// GenerateStream generates MNIST data stream. MNIST data is packed to binary
// using msgpack. Before unpacked structure is `map[string]interface{}`, images
// data is set with "data" key, and labels data is set with "target". The binary
// data is set with "mnist" key in a tuple.
//
// The MNIST data is randomized when random flag is true, a seed of randomizing
// is not fixed. And the data is separated by batch size. When a images data
// size is 60,000 and batch size is 100, then 600 (=60,000/100) tuples will be
// generated. A batch counter is set "batch_count"  key in a tuple.
//
// Output:
//  data.Map{
//    "batch_count": [a count number of batch] (data.Int),
//    "mnist":       [data separated by batch size] (data.Blob),
//  }
func (s *mnistDataSource) GenerateStream(ctx *core.Context, w core.Writer) error {
	perm := make([]int, s.dataSize, s.dataSize)
	for i := range perm {
		perm[i] = i
	}
	if s.randomFlag {
		ramdomPermutaion(perm)
	}

	for i := 0; i < s.dataSize; i++ {
		image := make(data.Array, len(s.data[i]))
		for j, f := range s.data[i] {
			image[j] = data.Float(f)
		}
		dm := data.Map{
			"label": data.Int(s.target[i]),
			"data":  image,
		}

		now := time.Now()
		tu := core.Tuple{
			Data:          dm,
			Timestamp:     now,
			ProcTimestamp: now,
			Trace:         []core.TraceEvent{},
		}
		err := w.Write(ctx, &tu)
		if err == core.ErrSourceRewound || err == core.ErrSourceStopped {
			return err
		}
	}

	ctx.Log().Info("all MNIST data has been streaming")
	return nil
}

// Stop stops generating stream. TODO forced stop
func (s *mnistDataSource) Stop(ctx *core.Context) error {
	return nil
}

type dataSource struct {
	path string
}

func (s *dataSource) reader() (*bufio.Reader, io.Closer, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, nil, err
	}
	r := bufio.NewReader(f)
	return r, f, nil
}

func ramdomPermutaion(perm []int) {
	for i := range perm {
		j := rand.Intn(i + 1)
		perm[i], perm[j] = perm[j], perm[i]
	}
}