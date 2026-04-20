package brooders

type Service interface {
	GetAll() ([]Brooder, error)
	GetByID(id uint) (*Brooder, error)
	Create(brooder *Brooder) error
	UpdateSensorData(id uint, data SensorUpdate) error
	UpdateActuators(id uint, data ActuatorUpdate) error
}

type BrooderService struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &BrooderService{repo: repo}
}

func (s *BrooderService) GetAll() ([]Brooder, error) {
	return s.repo.FindAll()
}

func (s *BrooderService) GetByID(id uint) (*Brooder, error) {
	return s.repo.FindByID(id)
}

func (s *BrooderService) Create(brooder *Brooder) error {
	return s.repo.Create(brooder)
}

func (s *BrooderService) UpdateSensorData(id uint, data SensorUpdate) error {
	return s.repo.UpdateSensorData(id, data)
}

func (s *BrooderService) UpdateActuators(id uint, data ActuatorUpdate) error {
	return s.repo.UpdateActuators(id, data)
}
