// @title           Tour Company API
// @version         1.0
// @description     Aпишка для управления туристической компанией (документация Swagger).
// @termsOfService  http://your-terms.com/terms/
// @contact.name    Тур Компани
// @contact.url     http://contact.company.com
// @contact.email   support@company.com
// @license.name    MIT
// @license.url     https://opensource.org/licenses/MIT
// @host            localhost:4000
// @BasePath        /

package main

import (
	_ "turcompany/docs"
	"turcompany/internal/app"
)

func main() {
	app.Run()
}
