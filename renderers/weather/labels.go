package main

// Display text is independent of data normalization, fixed dates and units.
// Additional translations can replace this dictionary without changing them.
type labels struct {
	Title, Today, Tomorrow, Now, Hourly, Weekly                                   string
	HighLow, Wind, Updated, Model, Attribution, Precipitation, DailyPrecipitation string
}

var englishLabels = labels{
	Title: "Weather", Today: "Today", Tomorrow: "Tomorrow", Now: "Now",
	Hourly: "Hourly Forecast", Weekly: "7-Day Forecast", HighLow: "High / Low",
	Precipitation: "Precip", DailyPrecipitation: "Max precipitation probability", Wind: "Wind", Updated: "Updated", Model: "Model",
	Attribution: "Weather data by Open-Meteo",
}

var englishWeatherNames = map[int]string{
	-1: "Unknown", 0: "Clear", 1: "Mostly clear", 2: "Partly cloudy", 3: "Overcast",
	45: "Fog", 48: "Fog", 51: "Drizzle", 53: "Drizzle", 55: "Drizzle",
	56: "Freezing drizzle", 57: "Freezing drizzle", 61: "Rain", 63: "Rain", 65: "Rain",
	66: "Freezing rain", 67: "Freezing rain", 71: "Snow", 73: "Snow", 75: "Snow", 77: "Snow",
	80: "Rain showers", 81: "Rain showers", 82: "Rain showers", 85: "Snow showers", 86: "Snow showers",
	95: "Thunderstorm", 97: "Heavy thunderstorm", 96: "Thunder / hail", 99: "Thunder / hail",
}
