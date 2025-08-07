-- Query to find trip with greatest distance for documentation sample
SELECT 
    id,
    session_id,
    user_id,
    train_name,
    train_number,
    total_distance_km,
    duration_seconds,
    start_latitude,
    start_longitude,
    end_latitude,
    end_longitude,
    started_at,
    completed_at,
    tracking_data,
    route_coordinates
FROM trips 
WHERE total_distance_km IS NOT NULL 
  AND total_distance_km > 0
  AND tracking_data IS NOT NULL
  AND route_coordinates IS NOT NULL
ORDER BY total_distance_km DESC 
LIMIT 3;