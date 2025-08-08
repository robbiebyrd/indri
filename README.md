# Overview

# Technical Details

## Data Flow

1. A new user must register
   ```json
   {
      "action": "register", 
      "email": "user@example.com", 
      "password": "SuperSecret",
      "name": "A User",
      "displayName": "!!u532!!"
   }
   ```
2. A user must log in using an email/password combination.
   ```json
   {
      "action": "login", 
      "email": "user@example.com", 
      "password": "SuperSecret"
   }
   ```
   *Note: Messages sent to Indri must include the `action` attribute. Message Handlers use the `action` attribute to
   appropriately route messages.*

3. A Session will be generated. A Session connects a User profile to a websocket connection and saves key metadata so
   that in the event a user is disconnected from a Session, they can quickly re-connect. The single required key in a
   Session object is the UserID; other keys, like Game ID and Team ID, are added to the session later. The server
   responds with the Session ID and the User object.
   ```json
   {
       "authenticated": true,
       "sessionId": "0000000000000000000000s1",
       "user": {
           "id": "0000000000000000000000u1",
           "createdAt": "1982-09-04T15:24:14.194Z",
           "updatedAt": "2017-11-03T15:24:14.194Z",
           "email": "user@example.com",
           "name": "A User",
           "displayName": "!!u532!!",
           "score": 0
       }
   }
   ```
4. Once a user has logged in and a Session has been created, the user can either `inquire` about games that are
   currently open, `create` a new game, or `join` a game already in progress. To join a game, you need a Game Code,
   or a string that represents a game.
5. To create a game, the user sends a creat

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



