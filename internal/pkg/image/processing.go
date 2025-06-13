package image

import (
	"github.com/h2non/bimg"
)

// ProcessImage processes an image to a specified height and quality
func ProcessImage(imageData []byte, height, quality int) ([]byte, error) {
	options := bimg.Options{
		Height:       height,
		Quality:      quality,
		Interpolator: bimg.Bicubic,
		Force:        false,
	}

	return bimg.NewImage(imageData).Process(options)
}

// IsSizeValid checks if an image is within the specified dimensions
func IsSizeValid(imageData []byte, width, height int) (bool, error) {
	size, err := bimg.NewImage(imageData).Size()
	if err != nil {
		return false, err
	}
	return size.Width <= width && size.Height <= height, nil
}

// ConvertToWebp converts an image to webp format
func ConvertToWebp(imageData []byte) ([]byte, error) {
	return bimg.NewImage(imageData).Convert(bimg.WEBP)
}
