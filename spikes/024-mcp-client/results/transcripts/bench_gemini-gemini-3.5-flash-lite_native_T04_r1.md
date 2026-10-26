# bench_gemini-gemini-3.5-flash-lite_native_T04_r1

**Prompt:** In the scene open in VISO, hide the box named Spare, but don't delete it.

Check: ok true 

Scene after:

```
[get_scene output trimmed to its scene index; the rest is Viso's text, not committed]
- Crate (box) pos [-1.5, 0.5, 0] size [1, 1, 1]  id:4a630e19-be76-453e-bc94-180a90127b3a
    #6366f1 metal 0 rough 0.6
- Spare (box) pos [0, 0.5, 0]  id:5ea35c7c-4b73-4fbd-8860-2a08e26ddbc3
    #6366f1 metal 0 rough 0.6 · hidden
- Lamp (cylinder) pos [1.5, 0.5, 0] size [1, 1, 1]  id:28fac475-cb9b-4b33-87ce-e04e83680e92
    #6366f1 metal 0 rough 0.6
```

Requests 3 · prompt tokens 41156 (cached 33353) · output 272 · first request 13602 tokens, 0 tools · 3s

## Calls

1. `viso_get_scene` {}
   → [get_scene output, Viso's text, not committed; the scene after the run is below]
2. `viso_set_visibility` {"id":"5ea35c7c-4b73-4fbd-8860-2a08e26ddbc3","visible":false}
   → Hid "Spare" (box, id:5ea35c7c-4b73-4fbd-8860-2a08e26ddbc3)

## Cards

- get_scene {} → always 
- set_visibility {"id":"5ea35c7c-4b73-4fbd-8860-2a08e26ddbc3","visible":false} → once 

## Answer

I have hidden the box named "Spare" in your scene.
