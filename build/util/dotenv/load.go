package dotenv

import (
	"os"

	"github.com/goyek/goyek/v2"
	"github.com/joho/godotenv"
)

func Load(next goyek.Executor) goyek.Executor {
	return func(in goyek.ExecuteInput) error {
		if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
			return err
		}

		return next(in)
	}
}
