package kit

import "context"

func (r *Repository) GetKits(ctx context.Context) ([]string, error) {
	return []string{
		"Рыцарь",
	}, nil
}
