module github.com/KantapatSg/golang-essential-3/services/notification-service

go 1.23.4

require (
	github.com/KantapatSg/golang-essential-3/contracts v0.0.0
	github.com/google/uuid v1.6.0
	github.com/segmentio/kafka-go v0.4.47
	google.golang.org/grpc v1.67.1
	gorm.io/driver/postgres v1.5.9
	gorm.io/gorm v1.25.12
)
replace github.com/KantapatSg/golang-essential-3/contracts => ../../contracts
