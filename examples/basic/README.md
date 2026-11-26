# Basic Demo

It's a basic demo of Castor web API.

## Endpoints

| Verb |  Path  | Parameters | Description |
|------|--------|--------|-------------|
| GET  | /      | None       | Entry point |
| GET  | /ping  | None       | Heart beat endpoint |
| GET  | /greet/`{name}` | `name` | Gretting endpoint |
| GET  | /contacts | None | List of contacts |
| GET  | /contacts/`{id}` | `id` | Display the contact by ID or return not found |

## Testing

```shell
curl -i "http://localhost:3000"
// output -> {"status":"OK", "message":"Castor!"}

curl -i "http://localhost:3000/ping"
// output -> {"status":"ok"}

curl -i "http://localhost:3000/greet/Melissa"
// output -> {"message":"Hey Melissa!"}
```
