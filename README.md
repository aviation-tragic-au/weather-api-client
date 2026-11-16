# weather-api-client

[![Lint](https://github.com/aviation-tragic-au/weather-api-client/actions/workflows/lint.yml/badge.svg)](https://github.com/aviation-tragic-au/weather-api-client/actions/workflows/lint.yml) [![Test](https://github.com/aviation-tragic-au/weather-api-client/actions/workflows/test.yml/badge.svg)](https://github.com/aviation-tragic-au/weather-api-client/actions/workflows/test.yml)

Provides a client for the Weather API.

https://www.weatherapi.com/

## Install

```sh
go get github.com/aviation-tragic-au/weather-api-client
```

## Usage

```go
import (
    "context"
    "errors"
    "log"

    weatherapi "github.com/aviation-tragic-au/weather-api-client"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()

    // URL defaults to https://api.weatherapi.com/v1. Set KeyFunc instead
    // of Key to look the key up on every request (to pick up a key changed
    // at runtime), and HTTPClient to use your own transport or timeout (the
    // default times out after 10 seconds).
    client, err := weatherapi.NewClient(logger, weatherapi.Config{
        Key: "your-api-key",
    })
    if err != nil {
        log.Fatal(err)
    }

    weather, err := client.GetCurrentForecast(context.Background(), -27.3842, 153.1175)
    var apiErr *weatherapi.APIError
    switch {
    case errors.As(err, &apiErr):
        log.Fatalf("WeatherAPI said no: %d %s", apiErr.Code, apiErr.Message)
    case err != nil:
        log.Fatal(err)
    }

    log.Printf("%s: %.1f°C, %s", weather.Location.Name, weather.Current.TempCelsius, weather.Current.Condition.Text)
}
```

A non-200 response is an `*APIError` carrying the HTTP status and
WeatherAPI's own [error code](https://www.weatherapi.com/docs/#intro-error-codes)
and message.

## Contributing

1. Fork it
2. Create your feature branch (`git checkout -b my-new-feature`)
3. Commit your changes (`git commit -am 'Add some feature'`)
4. Push to the branch (`git push origin my-new-feature`)
5. Create a new Pull Request

## License

Copyright 2026 Dean Stalker

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
