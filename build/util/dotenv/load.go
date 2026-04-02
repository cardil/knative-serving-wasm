package dotenv

import (
	"os"

	"github.com/goyek/goyek/v2"
	"github.com/joho/godotenv"
)

func Load(next goyek.Executor) goyek.Executor {
	return func(in goyek.ExecuteInput) error {
		for _, f := range []string{"user.env", ".env"} {
			if err := godotenv.Load(f); err != nil && !os.IsNotExist(err) {
				return err
			}
		}

		return next(in)
	}
}
