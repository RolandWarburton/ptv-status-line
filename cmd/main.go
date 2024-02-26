package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	app "github.com/rolandwarburton/ptv-status-line/pkg"
	"github.com/urfave/cli/v2"
)

func convertDatesToTimezone[Type interface{}](data []Type, timezone string) ([]Type, error) {
	var location *time.Location
	var err error
	if timezone == "UTC" {
		location = time.UTC
	} else {
		location, err = time.LoadLocation(timezone)
		if err != nil {
			fmt.Println("Error loading location:", err)
			return nil, err
		}
	}

	slice := reflect.ValueOf(data)

	// check a slice is being given
	if slice.Kind() != reflect.Slice {
		return nil, errors.New("a slice was not given")
	}

	// create a copy of the data
	// we can overwrite these elements later as we iterate over the slice
	newData := reflect.MakeSlice(reflect.TypeOf(data), slice.Len(), slice.Len())

	//for each element in the reflected slice
	for i := 0; i < slice.Len(); i++ {
		elem := slice.Index(i)

		// skip if this is not an interface that we can print
		if !elem.CanInterface() {
			newData.Index(i).Set(elem)
			continue
		}

		// get the field in this interface
		field := elem.FieldByName("ScheduledDepartureUTC")

		// skip if this is not the ScheduledDepartureUTC field
		if !field.IsValid() {
			newData.Index(i).Set(elem)
			continue
		}

		// // parse the date string into a date
		layout := "02-01-2006 03:04 PM"
		departureDate, err := time.Parse(layout, field.Interface().(string))
		if err != nil {
			fmt.Println(err)
			field.SetString("date conversion failed")
			newData.Index(i).Set(elem)
			continue
		}

		// convert the timezone
		departureDateTZ := departureDate.In(location)
		field.SetString(departureDateTZ.Format("02-01-2006 03:04 PM"))

		// overwrite the newData interface reflection with this new element
		newData.Index(i).Set(elem)
	}

	// reconstruct the newData reflection as the initial data type
	var result []Type
	for i := 0; i < newData.Len(); i++ {
		result = append(result, newData.Index(i).Interface().(Type))
	}

	return result, nil
}

func prettyPrint[Type interface{}](data []Type, timezone string) {
	// convert the date to the specified timezone
	data, _ = convertDatesToTimezone[Type](data, timezone)

	// pretty print
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(jsonData))
}

func writeToJSONFile(data any) {
	jsonData, _ := json.MarshalIndent(data, "", "  ")

	// write the routes to a file
	file, _ := os.Create("routes.json")
	defer file.Close()
	file.Write(jsonData)
}

func printFormatted[Type any](data []Type, format string, delimiter string) {
	// format as a string
	// Example --format "RouteID RouteName"
	formatArgs := strings.Split(format, " ")
	result := ""
	for i, item := range data {
		for j, arg := range formatArgs {
			// dynamically access the fields of the Route
			val := reflect.ValueOf(item)
			field := val.FieldByName(arg)
			if field.IsValid() && j < len(formatArgs)-1 {
				result += fmt.Sprintf("%v%s", field.Interface(), delimiter)
			} else {
				result += fmt.Sprintf("%v", field.Interface())
			}
		}
		if i < len(data)-1 {
			result += "\n"
		}
	}
	fmt.Println(result)

}

func routeAction(cCtx *cli.Context, format string, delimiter string) error {
	// get routes
	routes, _ := app.GetRoutes(cCtx.Args().First())

	// guard against no routes
	if len(routes) == 0 {
		fmt.Println("[]")
		return nil
	}

	// if not formatting print as JSON
	if !cCtx.IsSet("format") {
		jsonData, _ := json.MarshalIndent(routes, "", "  ")
		fmt.Println(string(jsonData))
		return nil
	}

	printFormatted[app.Route](routes, format, delimiter)
	return nil
}

func stopsAction(cCtx *cli.Context, stopName string, format string, delimiter string) error {
	// ensure a route ID is given
	stopID := cCtx.Args().First()
	if stopID == "" {
		return errors.New("please specify a route ID")
	}
	var v int
	var err error
	if v, err = strconv.Atoi(stopID); err != nil {
		return errors.New("please specify a valid route ID number")
	}

	// get the stops
	stops, err := app.GetStops(v, "", stopName)
	if err != nil {
		fmt.Println(err)
		return errors.New("failed to get routes")
	}

	// print as json if no formatting is given
	if format == "" {
		jsonData, _ := json.MarshalIndent(stops, "", "  ")
		fmt.Println(string(jsonData))
		return nil
	}

	printFormatted[app.Stop](stops, format, delimiter)

	return nil
}

func departuresAction(_ *cli.Context, routeID int, stopID int, direction int, departuresCount int, format string, delimiter string, timezone string) error {
	// get the departures for a stop on a route
	departures, err := app.GetDepartures(stopID, routeID, "")
	if err != nil {
		fmt.Println(err)
		return errors.New("failed to get departures")
	}

	// get the next N departures in a certain direction
	departuresTowardsDirection, err := app.GetNextDepartureTowards(departures, direction, departuresCount, timezone)
	if err != nil {
		fmt.Println(err)
		return errors.New("failed to get departures in specific direction")
	}

	nextDepartures := []app.Departure{}
	for i := 0; i < len(departuresTowardsDirection); i++ {
		if err == nil {
			layout := "2006-01-02T15:04:05Z"
			departureTime, err := time.Parse(layout, departuresTowardsDirection[i].ScheduledDepartureUTC)
			if err == nil {
				formattedTime := departureTime.Format("02-01-2006 03:04 PM")
				departuresTowardsDirection[i].ScheduledDepartureUTC = formattedTime
				nextDepartures = append(nextDepartures, departuresTowardsDirection[i])
			}
		}
	}

	if format == "" {
		prettyPrint[app.Departure](departuresTowardsDirection, timezone)
		return nil
	}

	printFormatted[app.Departure](nextDepartures, format, delimiter)
	return nil
}

func directionsAction(cCtx *cli.Context, format string, delimiter string) error {
	arg1 := cCtx.Args().First()
	if arg1 == "" {
		return errors.New("route ID not provided")
	}
	var routeID int
	var err error
	if routeID, err = strconv.Atoi(arg1); err != nil {
		return errors.New("failed to parse route ID")
	}
	directions, _ := app.GetDirections(routeID)
	// print as json if no formatting is given
	if format == "" {
		prettyPrint(directions, "Australia/Sydney")
		return nil
	}

	// format as a string
	printFormatted[app.Direction](directions, format, delimiter)
	return nil
}

func main() {
	var format string
	var delimiter string
	var stopName string
	var departuresCount int
	var routeID int
	var stopID int
	var directionID int
	var timezone string

	flags := []cli.Flag{
		&cli.StringFlag{
			Name:        "format",
			Value:       "",
			Usage:       "format the output",
			Destination: &format,
		},
		&cli.StringFlag{
			Name:        "delimiter",
			Value:       " ",
			Usage:       "delimiter between format arguments",
			Destination: &delimiter,
		},
		&cli.StringFlag{
			Name:        "timezone",
			Value:       "Australia/Sydney",
			Usage:       "specify timezone for dates",
			Destination: &timezone,
		},
	}

	app := &cli.App{
		Name:                 "ptv-status-line",
		EnableBashCompletion: true,
		Commands: []*cli.Command{
			{
				Name:  "routes",
				Usage: "explore routes",
				Flags: flags,
				Action: func(c *cli.Context) error {
					return routeAction(c, format, delimiter)
				},
			},
			{
				Name:  "stops",
				Usage: "explore stops",
				Flags: append(flags, &cli.StringFlag{
					Name:        "stop",
					Value:       "",
					Usage:       "Filter a specific stop by the station name",
					Destination: &stopName,
				}),
				Action: func(c *cli.Context) error {
					return stopsAction(c, stopName, format, delimiter)
				},
			},
			{
				Name:  "departures",
				Usage: "explore stops",
				Flags: append(flags,
					&cli.IntFlag{
						Name:        "count",
						Value:       -1,
						Usage:       "The next N trains departing",
						Destination: &departuresCount,
					},
					&cli.IntFlag{
						Name:        "route",
						Value:       -1,
						Usage:       "The route ID",
						Destination: &routeID,
					},
					&cli.IntFlag{
						Name:        "stop",
						Value:       -1,
						Usage:       "The stop ID",
						Destination: &stopID,
					},
					&cli.IntFlag{
						Name:        "direction",
						Value:       -1,
						Usage:       "The direction ID",
						Destination: &directionID,
					},
				),
				Action: func(c *cli.Context) error {
					return departuresAction(c, routeID, stopID, directionID, departuresCount, format, delimiter, timezone)
				},
			},
			{
				Name:  "directions",
				Usage: "explore directions",
				Flags: flags,
				Action: func(c *cli.Context) error {
					directionsAction(c, format, delimiter)
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
