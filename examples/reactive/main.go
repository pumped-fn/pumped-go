package main

import (
	"context"
	"fmt"
	"time"

	"github.com/pumped-fn/pumped-go/pkg/core"
)

// UserProfile represents a user profile
type UserProfile struct {
	Name  string
	Email string
	Age   int
}

// UserPreferences represents user preferences
type UserPreferences struct {
	Theme         string
	Notifications bool
}

// EnhancedUserProfile combines profile and preferences
type EnhancedUserProfile struct {
	Profile     UserProfile
	Preferences UserPreferences
	LastActive  time.Time
}

func main() {
	// Create a user profile executor
	userProfile := core.Provide(func(ctrl core.Controller) (UserProfile, error) {
		fmt.Println("Initializing user profile")
		return UserProfile{
			Name:  "John Doe",
			Email: "john@example.com",
			Age:   30,
		}, nil
	})

	// Create a user preferences executor
	userPreferences := core.Provide(func(ctrl core.Controller) (UserPreferences, error) {
		fmt.Println("Initializing user preferences")
		return UserPreferences{
			Theme:         "dark",
			Notifications: true,
		}, nil
	})

	// Create an enhanced user profile executor that depends on profile and preferences
	// Using DeriveTyped for strongly typed dependencies
	enhancedProfile := core.DeriveTyped(
		userProfile,
		userPreferences,
		func(profile UserProfile, prefs UserPreferences, ctrl core.Controller) (EnhancedUserProfile, error) {
			fmt.Println("Computing enhanced profile")
			return EnhancedUserProfile{
				Profile:     profile,
				Preferences: prefs,
				LastActive:  time.Now(),
			}, nil
		},
	)

	// Create a reactive version of the enhanced profile
	reactiveProfile := enhancedProfile.Reactive()

	// Create a scope
	scope := core.CreateScope()

	// Get an accessor for the reactive profile
	profileAccessor, err := scope.ResolveAccessor(context.Background(), reactiveProfile)
	if err != nil {
		fmt.Printf("Error resolving profile accessor: %v\n", err)
		return
	}

	// Subscribe to profile changes
	cleanup := profileAccessor.Subscribe(func(profile EnhancedUserProfile) {
		fmt.Printf("Profile updated: %s, Theme: %s, Last Active: %s\n",
			profile.Profile.Name,
			profile.Preferences.Theme,
			profile.LastActive.Format(time.RFC3339),
		)
	})

	// Initial profile
	initialProfile := profileAccessor.Get()
	fmt.Printf("Initial profile: %s, Theme: %s, Last Active: %s\n",
		initialProfile.Profile.Name,
		initialProfile.Preferences.Theme,
		initialProfile.LastActive.Format(time.RFC3339),
	)

	// Update the user profile
	err = scope.Update(context.Background(), userProfile, UserProfile{
		Name:  "Jane Doe",
		Email: "jane@example.com",
		Age:   28,
	})
	if err != nil {
		fmt.Printf("Error updating profile: %v\n", err)
		return
	}

	// Wait for the subscription to be triggered
	time.Sleep(100 * time.Millisecond)

	// Update the user preferences
	err = scope.Update(context.Background(), userPreferences, UserPreferences{
		Theme:         "light",
		Notifications: false,
	})
	if err != nil {
		fmt.Printf("Error updating preferences: %v\n", err)
		return
	}

	// Wait for the subscription to be triggered
	time.Sleep(100 * time.Millisecond)

	// Cleanup the subscription
	cleanup()

	// Dispose the scope
	err = scope.Dispose(context.Background())
	if err != nil {
		fmt.Printf("Error disposing scope: %v\n", err)
		return
	}
}

