# Overview

# Technical Details

## Data Models

All indri game data is stored in MongoDB. Collections exist for games and players.

### Game

The game model holds the bulk of information about a game in indri. Each Game holds the following data:

| Field | Data Type | Description                                                                                                     |
|-------|-----------|-----------------------------------------------------------------------------------------------------------------|
| Code  | string    | The Game *Code*, sometimes referred to as the *Game ID* or *Room Name*, among others.                           |
| Teams | object    | An key/value object, with the key being the TeamID and the object containing data about the team.               |
| Stage | object    | The stage represents the visual part of the game, and is made up of scenes which can be triggered in any order. |


### Stage

The Stage holds all the data needed to visually render the game. 

While stages contain some metadata, they primarily hold a contain a group of Scenes (see Scenes).

As well, each Stage has access to three specific data stores: a public data store, a private data store, and a
Player-specific data store.


| Field       | Description                                                                                                                                   |
|-------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| PublicData  | Data stored here will be sent to all players of the game. This is useful for global data you want to have available to all players in a Game. |
| PrivateData | Data stored here is private and will not be sent to players. Store any data you may want to use later, but keep private for now.              |
| PlayerData  | Data stored here is only available to a given player in a game. This data is not broadcast to the game, but is sent to the Player's session.  |


### Scenes



