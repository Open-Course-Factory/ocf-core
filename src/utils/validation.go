package utils

func ValidateNotOwner(userID, ownerUserID, entityType string) error {
	if userID == ownerUserID {
		return ErrCannotRemoveOwner(entityType)
	}
	return nil
}

func ValidateLimitNotReached(current, limit int, entityType string) error {
	if limit == -1 {
		return nil // Unlimited
	}
	if current >= limit {
		return ErrLimitReached(entityType, limit)
	}
	return nil
}
